// +build traceview

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

/*
#cgo LDFLAGS: -loboe
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"
import (
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Global configuration settings
type oboeSettings struct{ settingsCfg C.oboe_settings_cfg_t }

var globalSettings oboeSettings
var emptyCString, inXTraceCString *C.char
var oboeVersion string
var layerCache *cStringCache

// Initialize Traceview C instrumentation library ("oboe"):
func init() {
	C.oboe_init()
	readEnvSettings()

	// To save on malloc/free, preallocate empty & "non-empty" CStrings
	emptyCString = C.CString("")
	inXTraceCString = C.CString("in_xtrace")
	oboeVersion = C.GoString(C.oboe_config_get_version_string())
	layerCache = newCStringCache()
}

func readEnvSettings() {
	// Configure tracing mode setting using environment variable
	C.oboe_settings_cfg_init(&globalSettings.settingsCfg)
	mode := strings.ToLower(os.Getenv("GO_TRACEVIEW_TRACING_MODE"))
	switch mode {
	case "always":
		fallthrough
	default:
		globalSettings.settingsCfg.tracing_mode = C.OBOE_TRACE_ALWAYS
	case "through":
		globalSettings.settingsCfg.tracing_mode = C.OBOE_TRACE_THROUGH
	case "never":
		globalSettings.settingsCfg.tracing_mode = C.OBOE_TRACE_NEVER
	}

	if level := os.Getenv("APPOPTICS_DEBUG_LEVEL"); level != "" {
		if i, err := strconv.Atoi(level); err == nil {
			debugLevel = DebugLevel(i)
		} else {
			OboeLog(WARNING, "The debug level should be an integer.")
		}
	}
}

var initMessageOnce sync.Once

const initVersion = 1
const initLayer = "go"

func sendInitMessage() {
	ctx := newContext()
	if c, ok := ctx.(*oboeContext); ok {
		c.reportEvent(LabelEntry, initLayer, false,
			"__Init", 1,
			"Go.Version", runtime.Version(),
			"Go.Oboe.Version", initVersion,
			"Oboe.Version", oboeVersion,
		)
		c.ReportEvent(LabelExit, initLayer)
	}
}

var rateCounterDefaultRate = 5.0
var rateCounterDefaultSize = 3.0
var counterIntervalSecs = 30

type rateCounter struct {
	ratePerSec          float64
	capacity, available float64
	last                time.Time
	lock                sync.Mutex
	rateCounts
}
type rateCounts struct{ requested, sampled, limited, traced, through int64 }

func newRateCounter(ratePerSec, size float64) *rateCounter {
	return &rateCounter{ratePerSec: ratePerSec, capacity: size, available: size, last: time.Now()}
}

func (b *rateCounter) Count(sampled, hasMetadata bool) bool {
	atomic.AddInt64(&b.requested, 1)
	if hasMetadata {
		atomic.AddInt64(&b.through, 1)
	}
	if !sampled {
		return sampled
	}
	atomic.AddInt64(&b.sampled, 1)
	if ok := b.consume(1); !ok {
		atomic.AddInt64(&b.limited, 1)
		return false
	}
	atomic.AddInt64(&b.traced, 1)
	return sampled
}

func (b *rateCounter) consume(size float64) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.update(time.Now())
	if b.available >= size {
		b.available -= size
		return true
	}
	return false
}

func (b *rateCounter) update(now time.Time) {
	if b.available < b.capacity { // room for more tokens?
		delta := now.Sub(b.last) // calculate duration since last check
		b.last = now             // update time of last check
		if delta <= 0 {          // return if no delta or time went "backwards"
			return
		}
		newTokens := b.ratePerSec * delta.Seconds()               // # tokens generated since last check
		b.available = math.Min(b.capacity, b.available+newTokens) // add new tokens to bucket, but don't overfill
	}
}

func (b *rateCounter) Flush() *rateCounts {
	return &rateCounts{
		requested: atomic.SwapInt64(&b.requested, 0),
		sampled:   atomic.SwapInt64(&b.sampled, 0),
		limited:   atomic.SwapInt64(&b.limited, 0),
		traced:    atomic.SwapInt64(&b.traced, 0),
		through:   atomic.SwapInt64(&b.through, 0),
	}
}

func getNextInterval(now time.Time) time.Duration {
	return time.Duration(counterIntervalSecs)*time.Second -
		(time.Duration(now.Second()%counterIntervalSecs)*time.Second +
			time.Duration(now.Nanosecond())*time.Nanosecond)
}

func appendCount(e *event, counts map[string]*rateCounts, name string, f func(*rateCounts) int64) {
	if len(counts) == 0 {
		return
	}
	for layer, c := range counts {
		startObject := bsonAppendStartObject(&e.bbuf, name)
		e.AddInt64(layer, f(c))
		bsonAppendFinishObject(&e.bbuf, startObject)
	}
}

func oboeSampleRequest(layer, xtraceHeader string) (bool, int, int) {
	//	if usingTestReporter {
	//		if r, ok := globalReporter().(*TestReporter); ok {
	//			return r.ShouldTrace, 1000000, 2 // trace tests
	//		}
	//	}
	initMessageOnce.Do(sendInitMessage)

	var sampleRate, sampleSource C.int
	cachedLayer := layerCache.Get(layer)
	var cxt *C.char
	if xtraceHeader == "" {
		// common case, where we are the entry layer
		cxt = emptyCString
	} else {
		// use const "in_xtrace" arg, oboe_sample_request only checks if it is non-empty
		cxt = inXTraceCString
	}

	sample := int(C.oboe_sample_request(cachedLayer.name, cxt, &globalSettings.settingsCfg, &sampleRate, &sampleSource))
	sampled := cachedLayer.counter.Count(sample != 0, xtraceHeader != "")
	return sampled, int(sampleRate), int(sampleSource)
}
