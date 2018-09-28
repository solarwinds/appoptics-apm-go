// +build !disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package reporter

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
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

func newOboeSettings() *oboeSettings {
	return &oboeSettings{
		bucket: &tokenBucket{},
	}
}

// token bucket
type tokenBucket struct {
	ratePerSec float64
	capacity   float64
	available  float64
	last       time.Time
	lock       sync.Mutex
}

func (b *tokenBucket) setRate(r float64) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ratePerSec = r
}

func (b *tokenBucket) setCap(c float64) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.capacity = c
	if b.available > b.capacity {
		b.available = b.capacity
	}
}

func (b *tokenBucket) setAvail(a float64) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.available = a
	if b.available > b.capacity {
		b.available = b.capacity
	}
}

func (b *tokenBucket) avail() float64 {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.available
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

func init() {
	readEnvSettings()
	rand.Seed(time.Now().UnixNano())
}

func readEnvSettings() {
	// Configure tracing mode setting using environment variable
	mode := config.GetTracingMode()
	switch mode {
	case "always":
		fallthrough
	default:
		globalSettingsCfg.tracingMode = TRACE_ALWAYS
	case "never":
		globalSettingsCfg.tracingMode = TRACE_NEVER
	}
}

func sendInitMessage() {
	ctx := newContext(true)
	if c, ok := ctx.(*oboeContext); ok {
		// create new event from context
		e, err := c.newEvent("single", "go")
		if err != nil {
			log.Warningf("Error while creating the init message: %v", err)
			return
		}

		e.AddKV("__Init", 1)
		e.AddKV("Go.Version", utils.GoVersion())
		e.AddKV("Go.AppOptics.Version", utils.Version())

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

	if globalSettingsCfg.tracingMode == TRACE_NEVER {
		return false, 0, SAMPLE_SOURCE_NONE
	}

	var setting *oboeSettings
	var ok bool
	if setting, ok = getSetting(layer); !ok {
		// log.Debugf("Sampling disabled for %v until valid settings are retrieved.", layer)
		return false, 0, SAMPLE_SOURCE_NONE
	}

	var sampleRate int
	var sampleSource sampleSource
	retval := false
	doRateLimiting := false

	sampleRate = utils.Max(utils.Min(setting.value, maxSamplingRate), 0)

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

	// log.Debugf("Sampling with rate=%v, source=%v", sampleRate, sampleSource)
	return retval, sampleRate, sampleSource
}

func bytesToFloat64(b []byte) (float64, error) {
	if len(b) != 8 {
		return -1, fmt.Errorf("invalid length: %d", len(b))
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
}

func bytesToInt32(b []byte) (int32, error) {
	if len(b) != 4 {
		return -1, fmt.Errorf("invalid length: %d", len(b))
	}
	return int32(binary.LittleEndian.Uint32(b)), nil
}

func parseFloat64(args map[string][]byte, key string, fb float64) float64 {
	ret := fb
	if c, ok := args[key]; ok {
		v, err := bytesToFloat64(c)
		if err == nil && v >= 0 {
			ret = v
			log.Debugf("parsed %s=%f", key, v)
		} else {
			log.Warningf("parse error: %s=%f err=%v fallback=%f", key, v, err, fb)
		}
	}
	return ret
}

func parseInt32(args map[string][]byte, key string, fb int32) int32 {
	ret := fb
	if c, ok := args[key]; ok {
		v, err := bytesToInt32(c)
		if err == nil && v >= 0 {
			ret = v
			log.Debugf("parsed %s=%d", key, v)
		} else {
			log.Warningf("parse error: %s=%d err=%v fallback=%d", key, v, err, fb)
		}
	}
	return ret
}

func updateSetting(sType int32, layer string, flags []byte, value int64, ttl int64, args map[string][]byte) {
	ns := newOboeSettings()

	ns.timestamp = time.Now()
	ns.sType = settingType(sType)
	ns.flags = flagStringToBin(string(flags))
	ns.value = int(value)
	ns.ttl = ttl
	ns.layer = layer

	ns.bucket.capacity = parseFloat64(args, kvBucketCapacity, 0)
	ns.bucket.ratePerSec = parseFloat64(args, kvBucketRate, 0)

	key := oboeSettingKey{
		sType: settingType(sType),
		layer: layer,
	}

	globalSettingsCfg.lock.Lock()
	if s, ok := globalSettingsCfg.settings[key]; ok {
		ns.bucket.setAvail(s.bucket.avail())
	}
	globalSettingsCfg.settings[key] = ns
	globalSettingsCfg.lock.Unlock()
}

func resetSettings() {
	globalSettingsCfg.lock.Lock()
	defer globalSettingsCfg.lock.Unlock()

	flushRateCounts()
	globalSettingsCfg.settings = make(map[oboeSettingKey]*oboeSettings)
	readEnvSettings()
}

// OboeCheckSettingsTimeout checks and deletes expired settings
func OboeCheckSettingsTimeout() {
	globalSettingsCfg.checkSettingsTimeout()
}

func (sc *oboeSettingsCfg) checkSettingsTimeout() {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	ss := sc.settings
	for k, s := range ss {
		e := s.timestamp.Add(time.Duration(s.ttl) * time.Second)
		if e.Before(time.Now()) {
			delete(ss, k)
		}
	}
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

func removeSetting(layer string) {
	globalSettingsCfg.lock.Lock()
	defer globalSettingsCfg.lock.Unlock()

	key := oboeSettingKey{
		sType: TYPE_DEFAULT,
		layer: "",
	}

	delete(globalSettingsCfg.settings, key)
}

func hasDefaultSetting() bool {
	if _, ok := getSetting(""); ok {
		return true
	}
	return false
}

func shouldSample(sampleRate int) bool {
	retval := sampleRate == maxSamplingRate || rand.Intn(maxSamplingRate) <= sampleRate
	// log.Debugf("shouldSample(%v) => %v", sampleRate, retval)
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
