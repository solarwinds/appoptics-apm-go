// Copyright (C) 2016 Librato, Inc. All rights reserved.

package reporter

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/pkg/errors"
)

// Current settings configuration
type oboeSettingsCfg struct {
	settings map[oboeSettingKey]*oboeSettings
	lock     sync.RWMutex
	metrics.RateCounts
}
type oboeSettings struct {
	timestamp time.Time
	// the flags which may be modified through merging local settings.
	flags settingFlag
	// the original flags retrieved from the remote collector.
	originalFlags settingFlag
	// The sample rate. It could be the original value got from remote server
	// or a new value after negotiating with local config
	value int
	// The sample source after negotiating with local config
	source sampleSource
	ttl    int64
	layer  string
	bucket *tokenBucket
}

func (s *oboeSettings) hasOverrideFlag() bool {
	return s.originalFlags&FLAG_OVERRIDE != 0
}

func newOboeSettings() *oboeSettings {
	return &oboeSettings{
		bucket: globalTokenBucket,
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

func (b *tokenBucket) reset() {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.ratePerSec = 0
	b.capacity = 0
	b.available = 0
	b.last = time.Time{}
}

func (b *tokenBucket) setRateCap(rate, cap float64) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ratePerSec = rate
	b.capacity = cap

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

// The global token bucket. Trace decisions of all the requests are controlled
// by this single bucket.
//
// The rate and capacity will be initialized by the values fetched from the remote
// server, therefore it's initialized with only the default values.
var globalTokenBucket = &tokenBucket{}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func sendInitMessage() {
	if Closed() {
		log.Info(errors.Wrap(ErrReporterIsClosed, "send init message"))
		return
	}
	ctx := newContext(true)
	if c, ok := ctx.(*oboeContext); ok {
		// create new event from context
		e, err := c.newEvent("single", "go")
		if err != nil {
			log.Warningf("Error while creating the init message: %v", err)
			return
		}

		// we choose to ignore the errors
		_ = e.AddKV("__Init", 1)
		_ = e.AddKV("Go.Version", utils.GoVersion())
		_ = e.AddKV("Go.AppOptics.Version", utils.Version())

		_ = e.ReportStatus(c)
	}
}

func (b *tokenBucket) count(sampled, hasMetadata, rateLimit bool) bool {
	c := globalSettingsCfg
	c.RequestedInc()
	if hasMetadata {
		c.ThroughInc()
	}
	if !sampled {
		return sampled
	}
	c.SampledInc()
	if rateLimit {
		if ok := b.consume(1); !ok {
			c.LimitedInc()
			return false
		}
	}
	c.TracedInc()
	return sampled
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

func oboeSampleRequest(layer string, traced bool, url string) (bool, int, sampleSource, bool) {
	if usingTestReporter {
		if r, ok := globalReporter.(*TestReporter); ok {
			if !r.UseSettings {
				return r.ShouldTrace, 0, SAMPLE_SOURCE_NONE, true // trace tests
			}
		}
	}

	var setting *oboeSettings
	var ok bool
	if setting, ok = getSetting(layer); !ok {
		return false, 0, SAMPLE_SOURCE_NONE, false
	}

	retval := false
	doRateLimiting := false

	sampleRate, flags, source := mergeURLSetting(setting, url)

	if !traced {
		// A new request
		if flags&FLAG_SAMPLE_START != 0 {
			retval = shouldSample(sampleRate)
			if retval {
				doRateLimiting = true
			}
		}
	} else {
		// A traced request
		if flags&FLAG_SAMPLE_THROUGH_ALWAYS != 0 {
			retval = true
		} else if flags&FLAG_SAMPLE_THROUGH != 0 {
			retval = shouldSample(sampleRate)
		}
	}

	retval = setting.bucket.count(retval, traced, doRateLimiting)

	return retval, sampleRate, source, flags.Enabled()
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

// mergeLocalSetting follow the predefined precedence to decide which one to
// pick from: either the local configs or the remote ones, or the combination.
//
// Note: This function modifies the argument in place.
func mergeLocalSetting(remote *oboeSettings) *oboeSettings {
	if remote.hasOverrideFlag() && config.SamplingConfigured() {
		// Choose the lower sample rate and merge the flags
		if remote.value > config.GetSampleRate() {
			remote.value = config.GetSampleRate()
			remote.source = SAMPLE_SOURCE_FILE
		}
		remote.flags &= newTracingMode(config.GetTracingMode()).toFlags()
	} else if config.SamplingConfigured() {
		// Use local sample rate and tracing mode config
		remote.value = config.GetSampleRate()
		remote.flags = newTracingMode(config.GetTracingMode()).toFlags()
		remote.source = SAMPLE_SOURCE_FILE
	}
	return remote
}

// mergeURLSetting merges the service level setting (merged from remote and local
// settings) and the per-URL sampling flags, if any.
func mergeURLSetting(setting *oboeSettings, url string) (int, settingFlag, sampleSource) {
	if url == "" {
		return setting.value, setting.flags, setting.source
	}

	urlTracingMode := urls.getTracingMode(url)
	if urlTracingMode.isUnknown() {
		return setting.value, setting.flags, setting.source
	}

	flags := urlTracingMode.toFlags()
	source := SAMPLE_SOURCE_FILE

	if setting.hasOverrideFlag() {
		flags &= setting.originalFlags
	}

	return setting.value, flags, source
}

func adjustSampleRate(rate int64) int {
	if rate < 0 {
		log.Debugf("Invalid sample rate: %d", rate)
		return 0
	}

	if rate > maxSamplingRate {
		log.Debugf("Invalid sample rate: %d", rate)
		return maxSamplingRate
	}
	return int(rate)
}

func updateSetting(sType int32, layer string, flags []byte, value int64, ttl int64, args map[string][]byte) {
	ns := newOboeSettings()

	ns.timestamp = time.Now()
	ns.source = settingType(sType).toSampleSource()
	ns.flags = flagStringToBin(string(flags))
	ns.originalFlags = ns.flags
	ns.value = adjustSampleRate(value)
	ns.ttl = ttl
	ns.layer = layer

	rate := parseFloat64(args, kvBucketRate, 0)
	capacity := parseFloat64(args, kvBucketCapacity, 0)
	ns.bucket.setRateCap(rate, capacity)

	merged := mergeLocalSetting(ns)

	key := oboeSettingKey{
		sType: settingType(sType),
		layer: layer,
	}

	globalSettingsCfg.lock.Lock()
	globalSettingsCfg.settings[key] = merged
	globalSettingsCfg.lock.Unlock()
}

// Used for tests only
func resetSettings() {
	globalSettingsCfg.lock.Lock()
	defer globalSettingsCfg.lock.Unlock()

	globalSettingsCfg.FlushRateCounts()
	globalSettingsCfg.settings = make(map[oboeSettingKey]*oboeSettings)
	globalTokenBucket.reset()
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
