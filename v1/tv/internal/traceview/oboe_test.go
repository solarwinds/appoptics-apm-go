// +build !disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"bytes"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/mgo.v2/bson"

	g "github.com/librato/go-traceview/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

func newTokenBucket(ratePerSec, size float64) *tokenBucket {
	return &tokenBucket{ratePerSec: ratePerSec, capacity: size, available: size, last: time.Now()}
}

func TestInitMessage(t *testing.T) {
	r := SetTestReporter(true)

	sendInitMessage()
	r.Close(2)
	assertInitMessage(t, r.Bufs)
}
func assertInitMessage(t *testing.T, bufs [][]byte) {
	g.AssertGraph(t, bufs, 2, g.AssertNodeMap{
		{"go", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, 1, n.Map["__Init"])
			assert.Equal(t, initVersion, n.Map["Go.Oboe.Version"])
			assert.NotEmpty(t, n.Map["Go.Version"])
		}},
		{"go", "exit"}: {Edges: g.Edges{{"go", "entry"}}},
	})
}

func TestInitMessageUDP(t *testing.T) {
	assertUDPMode(t)

	var bufs [][]byte
	done := startTestUDPListener(t, &bufs, 2)
	sendInitMessage()
	<-done
	assertInitMessage(t, bufs)
}

func TestTokenBucket(t *testing.T) {
	b := newTokenBucket(5, 2)
	c := globalSettingsCfg
	consumers := 5
	iters := 100
	sendRate := 30 // test request rate of 30 per second
	sleepInterval := time.Second / time.Duration(sendRate)
	var wg sync.WaitGroup
	wg.Add(consumers)
	var dropped, allowed int64
	for j := 0; j < consumers; j++ {
		go func(id int) {
			perConsumerRate := newTokenBucket(15, 1)
			for i := 0; i < iters; i++ {
				sampled := perConsumerRate.consume(1)
				ok := b.count(sampled, true, true)
				if ok {
					//t.Logf("### OK   id %02d now %v last %v tokens %v", id, time.Now(), b.last, b.available)
					atomic.AddInt64(&allowed, 1)
				} else {
					//t.Logf("--- DROP id %02d now %v last %v tokens %v", id, time.Now(), b.last, b.available)
					atomic.AddInt64(&dropped, 1)
				}
				time.Sleep(sleepInterval)
			}
			wg.Done()
		}(j)
		time.Sleep(sleepInterval / time.Duration(consumers))
	}
	wg.Wait()
	t.Logf("TB iters %d allowed %v dropped %v limited %v", iters, allowed, dropped, c.limited)
	assert.True(t, (iters == 100 && consumers == 5))
	assert.True(t, (allowed == 20 && dropped == 480 && c.limited == 230 && c.traced == 20) ||
		(allowed == 19 && dropped == 481 && c.limited == 231 && c.traced == 19) ||
		(allowed == 18 && dropped == 482 && c.limited == 232 && c.traced == 18))
	assert.Equal(t, int64(500), c.requested)
	assert.Equal(t, int64(250), c.sampled)
	assert.Equal(t, int64(500), c.through)
}

func TestTokenBucketTime(t *testing.T) {
	b := newTokenBucket(5, 2)
	b.consume(1)
	assert.EqualValues(t, 1, b.available) // 1 available
	b.last = b.last.Add(time.Second)      // simulate time going backwards
	b.update(time.Now())
	assert.EqualValues(t, 1, b.available) // no new tokens added
	assert.True(t, b.consume(1))          // consume available token
	assert.False(t, b.consume(1))         // out of tokens
	assert.True(t, time.Now().After(b.last))
	time.Sleep(200 * time.Millisecond)
	assert.True(t, b.consume(1)) // another token available
}

func testLayerCount(count int64) interface{} {
	return bson.D{bson.DocElem{Name: testLayer, Value: count}}
}

func callShouldTraceRequest(total int, isTraced bool) (traced int) {
	for i := 0; i < total; i++ {
		if ok, _, _ := shouldTraceRequest(testLayer, isTraced); ok {
			traced++
		}
	}
	return traced
}

func TestSamplingRate(t *testing.T) {
	r := SetTestReporter(false)

	// set 2.5% sampling rate
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		25000, 120, argsToMap(1000000, 1000000, -1, -1))

	total := 1000000
	traced := callShouldTraceRequest(total, false)

	// make sure we're within 3% of our expected rate over 1,000,000 trials
	assert.InDelta(t, 2.5, float64(traced)*100/float64(total), 0.03)

	c := globalSettingsCfg
	assert.EqualValues(t, c.requested, total)
	assert.EqualValues(t, c.through, 0)
	assert.EqualValues(t, c.traced, traced)
	assert.EqualValues(t, c.sampled, traced)
	assert.EqualValues(t, c.limited, 0)

	r.Close(2)
}

func TestSampleTracingDisabled(t *testing.T) {
	r := SetTestReporter(true)

	total := 3
	globalSettingsCfg.tracingMode = TRACE_NEVER
	traced := callShouldTraceRequest(total, false)
	assert.EqualValues(t, 0, traced)

	globalSettingsCfg.tracingMode = TRACE_ALWAYS
	traced = callShouldTraceRequest(total, false)
	assert.EqualValues(t, 3, traced)

	reportingDisabled = true
	traced = callShouldTraceRequest(total, false)
	assert.EqualValues(t, 0, traced)

	r.Close(2)
}

func TestSampleNoValidSettings(t *testing.T) {
	r := SetTestReporter(false)

	total := 1

	var buf bytes.Buffer
	log.SetOutput(&buf)
	traced := callShouldTraceRequest(total, false)
	log.SetOutput(os.Stderr)
	assert.Contains(t, buf.String(), "Sampling disabled for go_test until valid settings are retrieved")
	assert.EqualValues(t, 0, traced)

	r.Close(2)
}

func TestSampleRateBoundaries(t *testing.T) {
	r := SetTestReporter(true)

	_, rate, _ := shouldTraceRequest(testLayer, false)
	assert.Equal(t, 1000000, rate)

	// check that max value doesn't go above 1000000
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000001, 120, argsToMap(1000000, 1000000, -1, -1))

	_, rate, _ = shouldTraceRequest(testLayer, false)
	assert.Equal(t, 1000000, rate)

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		0, 120, argsToMap(1000000, 1000000, -1, -1))

	_, rate, _ = shouldTraceRequest(testLayer, false)
	assert.Equal(t, 0, rate)

	// check that min value doesn't go below 0
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		-1, 120, argsToMap(1000000, 1000000, -1, -1))

	_, rate, _ = shouldTraceRequest(testLayer, false)
	assert.Equal(t, 0, rate)

	r.Close(2)
}

func TestSampleSource(t *testing.T) {
	r := SetTestReporter(true)

	_, _, source := shouldTraceRequest(testLayer, false)
	assert.Equal(t, SAMPLE_SOURCE_DEFAULT, source)

	r.resetSettings()
	_, _, source = shouldTraceRequest(testLayer, false)
	assert.Equal(t, SAMPLE_SOURCE_NONE, source)

	// we're currently only looking up default settings, so this should return NONE sample source
	updateSetting(int32(TYPE_LAYER), testLayer,
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
	_, _, source = shouldTraceRequest(testLayer, false)
	assert.Equal(t, SAMPLE_SOURCE_NONE, source)

	// as soon as we add the default settings back, we get a valid sample source
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
	_, _, source = shouldTraceRequest(testLayer, false)
	assert.Equal(t, SAMPLE_SOURCE_DEFAULT, source)

	r.Close(2)
}

func TestSampleFlags(t *testing.T) {
	r := SetTestReporter(false)
	c := globalSettingsCfg

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte(""),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
	ok, _, _ := shouldTraceRequest(testLayer, false)
	assert.False(t, ok)
	assert.EqualValues(t, 0, c.through)
	ok, _, _ = shouldTraceRequest(testLayer, true)
	assert.False(t, ok)
	assert.EqualValues(t, 1, c.through)

	r.resetSettings()
	c = globalSettingsCfg

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
	ok, _, _ = shouldTraceRequest(testLayer, false)
	assert.True(t, ok)
	assert.EqualValues(t, 0, c.through)
	ok, _, _ = shouldTraceRequest(testLayer, true)
	assert.False(t, ok)
	assert.EqualValues(t, 1, c.through)

	r.resetSettings()
	c = globalSettingsCfg

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
	ok, _, _ = shouldTraceRequest(testLayer, false)
	assert.False(t, ok)
	assert.EqualValues(t, 0, c.through)
	ok, _, _ = shouldTraceRequest(testLayer, true)
	assert.True(t, ok)
	assert.EqualValues(t, 1, c.through)

	r.resetSettings()
	c = globalSettingsCfg

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_THROUGH"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
	ok, _, _ = shouldTraceRequest(testLayer, false)
	assert.False(t, ok)
	assert.EqualValues(t, 0, c.through)
	ok, _, _ = shouldTraceRequest(testLayer, true)
	assert.True(t, ok)
	assert.EqualValues(t, 1, c.through)

	r.Close(2)
}

func TestSampleTokenBucket(t *testing.T) {
	r := SetTestReporter(true)
	c := globalSettingsCfg

	traced := callShouldTraceRequest(1, false)
	assert.EqualValues(t, 1, traced)
	assert.EqualValues(t, 1, c.traced)
	assert.EqualValues(t, 1, c.requested)
	assert.EqualValues(t, 0, c.limited)

	r.resetSettings()
	c = globalSettingsCfg

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START"),
		1000000, 120, argsToMap(0, 0, -1, -1))
	traced = callShouldTraceRequest(1, false)
	assert.EqualValues(t, 0, traced)
	assert.EqualValues(t, 0, c.traced)
	assert.EqualValues(t, 1, c.requested)
	assert.EqualValues(t, 1, c.limited)

	r.resetSettings()
	c = globalSettingsCfg

	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(16, 8, -1, -1))
	traced = callShouldTraceRequest(50, false)
	assert.EqualValues(t, 16, traced)
	assert.EqualValues(t, 16, c.traced)
	assert.EqualValues(t, 50, c.requested)
	assert.EqualValues(t, 34, c.limited)
	flushRateCounts()

	time.Sleep(1 * time.Second)

	traced = callShouldTraceRequest(50, false)
	assert.EqualValues(t, 8, traced)
	assert.EqualValues(t, 8, c.traced)
	assert.EqualValues(t, 50, c.requested)
	assert.EqualValues(t, 42, c.limited)

	r.Close(2)
}

//func TestMetrics(t *testing.T) {
//	// error sending metrics message: no reporting
//	r := SetTestReporter()
//
//	randReader = &errorReader{failOn: map[int]bool{0: true}}
//	sendMetricsMessage(r)
//	time.Sleep(100 * time.Millisecond)
//	r.Close(0)
//	assert.Len(t, r.Bufs, 0)
//
//	r = SetTestReporter()
//	randReader = &errorReader{failOn: map[int]bool{2: true}}
//	sendMetricsMessage(r)
//	time.Sleep(100 * time.Millisecond)
//	r.Close(0)
//	assert.Len(t, r.Bufs, 0)
//
//	randReader = rand.Reader // set back to normal
//}

//func assertGetNextInterval(t *testing.T, nowTime, expectedDur string) {
//	t0, err := time.Parse(time.RFC3339Nano, nowTime)
//	assert.NoError(t, err)
//	d0 := getNextInterval(t0)
//	d0e, err := time.ParseDuration(expectedDur)
//	assert.NoError(t, err)
//	assert.Equal(t, d0e, d0)
//	assert.Equal(t, 0, t0.Add(d0).Second()%counterIntervalSecs)
//}
//
//func TestGetNextInterval(t *testing.T) {
//	assertGetNextInterval(t, "2016-01-02T15:04:05.888-04:00", "24.112s")
//	assertGetNextInterval(t, "2016-01-02T15:04:35.888-04:00", "24.112s")
//	assertGetNextInterval(t, "2016-01-02T15:04:00.00-04:00", "30s")
//	assertGetNextInterval(t, "2016-08-15T23:31:30.00-00:00", "30s")
//	assertGetNextInterval(t, "2016-01-02T15:04:59.999999999-04:00", "1ns")
//	assertGetNextInterval(t, "2016-01-07T15:04:29.999999999-00:00", "1ns")
//}

//func TestSendMetrics(t *testing.T) {
//	if testing.Short() {
//		t.Skip("Skipping metrics periodic sender test")
//	}
//	// full periodic sender test: wait for next interval & report
//	r := SetTestReporter(time.Duration(30) * time.Second)
//	disableMetrics = false
//	go sendInitMessage()
//	d0 := getNextInterval(time.Now()) + time.Second
//	fmt.Printf("[%v] TestSendMetrics Sleeping for %v\n", time.Now(), d0)
//	time.Sleep(d0)
//	fmt.Printf("[%v] TestSendMetrics Closing\n", time.Now())
//	r.Close(4)
//	g.AssertGraph(t, r.Bufs, 4, g.AssertNodeMap{
//		{"go", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
//			assert.Equal(t, 1, n.Map["__Init"])
//			assert.Equal(t, initVersion, n.Map["Go.Oboe.Version"])
//			assert.NotEmpty(t, n.Map["Oboe.Version"])
//			assert.NotEmpty(t, n.Map["Go.Version"])
//		}},
//		{"go", "exit"}: {Edges: g.Edges{{"go", "entry"}}},
//		{metricsLayerName, "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
//			assert.Equal(t, "go", n.Map["ProcessName"])
//			assert.IsType(t, int64(0), n.Map["JMX.type=threadcount,name=NumGoroutine"])
//			assert.IsType(t, int64(0), n.Map["JMX.Memory:MemStats.Alloc"])
//			assert.True(t, len(n.Map) > 10)
//		}},
//		{metricsLayerName, "exit"}: {Edges: g.Edges{{metricsLayerName, "entry"}}},
//	})
//	stopMetrics <- struct{}{}
//	disableMetrics = true
//}

func TestOboeTracingMode(t *testing.T) {
	r := SetTestReporter(true)
	// make oboeSampleRequest think test reporter is not being used for these tests..
	//	usingTestReporter = false

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "ALWAYS")
	readEnvSettings()
	assert.EqualValues(t, globalSettingsCfg.tracingMode, 1) // C.OBOE_TRACE_ALWAYS

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "never")
	readEnvSettings()
	assert.EqualValues(t, globalSettingsCfg.tracingMode, 0) // C.OBOE_TRACE_NEVER
	ok, _, _ := oboeSampleRequest("myLayer", false)
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "")
	readEnvSettings()
	assert.EqualValues(t, globalSettingsCfg.tracingMode, 1)

	r.Close(0)
}
