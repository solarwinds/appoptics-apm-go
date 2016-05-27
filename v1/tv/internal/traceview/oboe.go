// +build traceview

// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

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
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var globalSettings settings
var emptyCString, inXTraceCString *C.char
var oboeVersion string
var layerCache *cStringCache

// Global configuration settings (sample rate, tracing mode.)
type settings struct {
	settings_cfg C.oboe_settings_cfg_t
}

// Initialize Traceview C instrumentation library ("oboe"):
func init() {
	C.oboe_init()
	globalReporter = newReporter()
	readEnvSettings()

	// To save on malloc/free, preallocate empty & "non-empty" CStrings
	emptyCString = C.CString("")
	inXTraceCString = C.CString("in_xtrace")
	oboeVersion = C.GoString(C.oboe_config_get_version_string())
	layerCache = newCStringCache()
}

func readEnvSettings() {
	// Configure tracing mode setting using environment variable
	C.oboe_settings_cfg_init(&globalSettings.settings_cfg)
	mode := strings.ToLower(os.Getenv("GO_TRACEVIEW_TRACING_MODE"))
	switch mode {
	case "always":
		fallthrough
	default:
		globalSettings.settings_cfg.tracing_mode = C.OBOE_TRACE_ALWAYS
	case "through":
		globalSettings.settings_cfg.tracing_mode = C.OBOE_TRACE_THROUGH
	case "never":
		globalSettings.settings_cfg.tracing_mode = C.OBOE_TRACE_NEVER
	}
}

var initVersion = 1
var initLayer = "go"
var initMessageOnce sync.Once

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

type rateCounter struct {
	ratePerSec                  float64
	capacity, available         float64
	last                        time.Time
	lock                        sync.Mutex
	requested, sampled, limited int64
	through                     int64
}

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

func oboeSampleRequest(layer, xtraceHeader string) (bool, int, int) {
	if usingTestReporter {
		if r, ok := globalReporter.(*TestReporter); ok {
			return r.ShouldTrace, 1000000, 2 // trace tests
		}
	}
	initMessageOnce.Do(sendInitMessage)

	var sampleRate, sampleSource C.int
	var clayer *C.char = layerCache.Get(layer)
	var cxt *C.char
	if xtraceHeader == "" {
		// common case, where we are the entry layer
		cxt = emptyCString
	} else {
		// use const "in_xtrace" arg, oboe_sample_request only checks if it is non-empty
		cxt = inXTraceCString
	}

	sample := int(C.oboe_sample_request(clayer, cxt, &globalSettings.settings_cfg, &sampleRate, &sampleSource))

	return sample != 0, int(sampleRate), int(sampleSource)
}
