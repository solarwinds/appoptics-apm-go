// +build !disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Current settings configuration
type oboeSettingsCfg struct {
	tracingMode tracingMode
	settings    map[oboeSettingKey]*oboeSettings
	lock        sync.RWMutex
	rateCounts
}
type oboeSettings struct {
	timestamp time.Time
	sType     settingType
	flags     settingFlag
	value     int
	ttl       int64
	layer     string
	bucket    *tokenBucket
}

// token bucket
type tokenBucket struct {
	ratePerSec float64
	capacity   float64
	available  float64
	last       time.Time
	lock       sync.Mutex
}

// The identifying keys for a setting
type oboeSettingKey struct {
	sType settingType
	layer string
}

// Global configuration settings
var globalSettingsCfg = &oboeSettingsCfg{
	settings: make(map[oboeSettingKey]*oboeSettings),
}

// Initialize Traceview C instrumentation library ("oboe"):
func init() {
	readEnvSettings()
	rand.Seed(time.Now().UnixNano())
}

func readEnvSettings() {
	// Configure tracing mode setting using environment variable
	mode := strings.ToLower(os.Getenv("APPOPTICS_TRACING_MODE"))
	switch mode {
	case "always":
		fallthrough
	default:
		globalSettingsCfg.tracingMode = TRACE_ALWAYS
	case "never":
		globalSettingsCfg.tracingMode = TRACE_NEVER
	}

	if level := os.Getenv("APPOPTICS_DEBUG_LEVEL"); level != "" {
		if i, err := strconv.Atoi(level); err == nil {
			debugLevel = DebugLevel(i)
		} else {
			OboeLog(WARNING, "The debug level should be an integer.")
		}
	}
}

const initVersion = 1

func sendInitMessage() {
	ctx := newContext(true)
	if c, ok := ctx.(*oboeContext); ok {
		// create new event from context
		e, err := c.newEvent("single", "go")
		if err != nil {
			OboeLog(ERROR, "Error while creating the init message")
		}

		e.AddKV("__Init", 1)
		e.AddKV("Go.Version", runtime.Version())
		e.AddKV("Go.Oboe.Version", strconv.Itoa(initVersion))

		e.ReportStatus(c)
	}
}

func (b *tokenBucket) count(sampled, hasMetadata, rateLimit bool) bool {
	c := globalSettingsCfg
	atomic.AddInt64(&c.requested, 1)
	if hasMetadata {
		atomic.AddInt64(&c.through, 1)
	}
	if !sampled {
		return sampled
	}
	atomic.AddInt64(&c.sampled, 1)
	if rateLimit {
		if ok := b.consume(1); !ok {
			atomic.AddInt64(&c.limited, 1)
			return false
		}
	}
	atomic.AddInt64(&c.traced, 1)
	return sampled
}

func flushRateCounts() *rateCounts {
	c := globalSettingsCfg
	return &rateCounts{
		requested: atomic.SwapInt64(&c.requested, 0),
		sampled:   atomic.SwapInt64(&c.sampled, 0),
		limited:   atomic.SwapInt64(&c.limited, 0),
		traced:    atomic.SwapInt64(&c.traced, 0),
		through:   atomic.SwapInt64(&c.through, 0),
	}
}

func (b *tokenBucket) consume(size float64) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.update(time.Now())
	if b.available >= size {
		b.available -= size
		return true
	}
	return false
}

func (b *tokenBucket) update(now time.Time) {
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

func oboeSampleRequest(layer string, traced bool) (bool, int, sampleSource) {
	if usingTestReporter {
		if r, ok := globalReporter.(*TestReporter); ok {
			if !r.UseSettings {
				return r.ShouldTrace, 0, SAMPLE_SOURCE_NONE // trace tests
			}
		}
	}

	if globalSettingsCfg.tracingMode == TRACE_NEVER || reportingDisabled {
		return false, 0, SAMPLE_SOURCE_NONE
	}

	var setting *oboeSettings
	var ok bool
	if setting, ok = getSetting(layer); !ok {
		OboeLog(DEBUG, fmt.Sprintf("Sampling disabled for %v until valid settings are retrieved.", layer))
		return false, 0, SAMPLE_SOURCE_NONE
	}

	var sampleRate int
	var sampleSource sampleSource
	retval := false
	doRateLimiting := false

	sampleRate = Max(Min(setting.value, maxSamplingRate), 0)

	switch setting.sType {
	case TYPE_DEFAULT:
		sampleSource = SAMPLE_SOURCE_DEFAULT
	case TYPE_LAYER:
		sampleSource = SAMPLE_SOURCE_LAYER
	default:
		sampleSource = SAMPLE_SOURCE_NONE
	}

	if !traced {
		// A new request
		if setting.flags&FLAG_SAMPLE_START != 0 {
			retval = shouldSample(sampleRate)
			if retval {
				doRateLimiting = true
			}
		}
	} else {
		// A traced request
		if setting.flags&FLAG_SAMPLE_THROUGH_ALWAYS != 0 {
			retval = true
		} else if setting.flags&FLAG_SAMPLE_THROUGH != 0 {
			retval = shouldSample(sampleRate)
		}
	}

	retval = setting.bucket.count(retval, traced, doRateLimiting)

	OboeLog(DEBUG, fmt.Sprintf("Sampling with rate=%v, source=%v", sampleRate, sampleSource))
	return retval, sampleRate, sampleSource
}

func updateSetting(sType int32, layer string, flags []byte, value int64, ttl int64, arguments *map[string][]byte) {
	globalSettingsCfg.lock.Lock()
	defer globalSettingsCfg.lock.Unlock()

	var bucketCapacity float64
	if c, ok := (*arguments)["BucketCapacity"]; ok {
		bits := binary.LittleEndian.Uint64(c)
		bucketCapacity = math.Float64frombits(bits)
	} else {
		bucketCapacity = 0
	}
	var bucketRatePerSec float64
	if c, ok := (*arguments)["BucketRate"]; ok {
		bits := binary.LittleEndian.Uint64(c)
		bucketRatePerSec = math.Float64frombits(bits)
	} else {
		bucketRatePerSec = 0
	}

	key := oboeSettingKey{
		sType: settingType(sType),
		layer: layer,
	}
	var setting *oboeSettings
	var ok bool
	if setting, ok = globalSettingsCfg.settings[key]; !ok {
		setting = &oboeSettings{
			bucket: &tokenBucket{},
		}
		globalSettingsCfg.settings[key] = setting
	}
	setting.timestamp = time.Now()
	setting.sType = settingType(sType)
	setting.flags = flagStringToBin(string(flags))
	setting.value = int(value)
	setting.ttl = ttl
	setting.layer = layer

	setting.bucket.lock.Lock()
	if bucketCapacity >= 0 {
		setting.bucket.capacity = bucketCapacity
	} else {
		setting.bucket.capacity = 0
		OboeLog(WARNING, fmt.Sprintf("Invalid bucket capacity (%v). Using %v.", bucketCapacity, 0))
	}
	if setting.bucket.available > setting.bucket.capacity {
		setting.bucket.available = setting.bucket.capacity
	}
	if bucketRatePerSec >= 0 {
		setting.bucket.ratePerSec = bucketRatePerSec
	} else {
		setting.bucket.ratePerSec = 0
		OboeLog(WARNING, fmt.Sprintf("Invalid bucket rate (%v). Using %v.", bucketRatePerSec, 0))
	}
	setting.bucket.lock.Unlock()
}

func resetSettings() {
	reportingDisabled = false
	flushRateCounts()
	globalSettingsCfg = &oboeSettingsCfg{
		settings: make(map[oboeSettingKey]*oboeSettings),
	}
	readEnvSettings()
}

func getSetting(layer string) (*oboeSettings, bool) {
	globalSettingsCfg.lock.RLock()
	defer globalSettingsCfg.lock.RUnlock()

	// for now only look up the default settings
	key := oboeSettingKey{
		sType: TYPE_DEFAULT,
		layer: "",
	}
	if setting, ok := globalSettingsCfg.settings[key]; ok {
		return setting, true
	}

	return nil, false
}

func shouldSample(sampleRate int) bool {
	retval := sampleRate == maxSamplingRate || rand.Intn(maxSamplingRate) <= sampleRate
	OboeLog(DEBUG, fmt.Sprintf("shouldSample(%v) => %v", sampleRate, retval))
	return retval
}

func flagStringToBin(flagString string) settingFlag {
	flags := settingFlag(0)
	if flagString != "" {
		for _, s := range strings.Split(flagString, ",") {
			switch s {
			case "OVERRIDE":
				flags |= FLAG_OVERRIDE
			case "SAMPLE_START":
				flags |= FLAG_SAMPLE_START
			case "SAMPLE_THROUGH":
				flags |= FLAG_SAMPLE_THROUGH
			case "SAMPLE_THROUGH_ALWAYS":
				flags |= FLAG_SAMPLE_THROUGH_ALWAYS
			}
		}
	}
	return flags
}
