// +build traceview

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"crypto/rand"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/mgo.v2/bson"

	g "github.com/librato/go-traceview/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

func TestInitMessage(t *testing.T) {
	r := SetTestReporter()

	sendInitMessage()
	r.Close(2)
	assertInitMessage(t, r.Bufs)
}
func assertInitMessage(t *testing.T, bufs [][]byte) {
	g.AssertGraph(t, bufs, 2, g.AssertNodeMap{
		{"go", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, 1, n.Map["__Init"])
			assert.Equal(t, initVersion, n.Map["Go.Oboe.Version"])
			assert.NotEmpty(t, n.Map["Oboe.Version"])
			assert.NotEmpty(t, n.Map["Go.Version"])
		}},
		{"go", "exit"}: {Edges: g.Edges{{"go", "entry"}}},
	})
}

func TestInitMessageUDP(t *testing.T) {
	var bufs [][]byte
	done := startTestUDPListener(t, &bufs, 2)
	sendInitMessage()
	<-done
	assertInitMessage(t, bufs)
}

func TestOboeRateCounter(t *testing.T) {
	b := newRateCounter(5, 2)
	consumers := 5
	iters := 100
	sendRate := 30 // test request rate of 30 per second
	sleepInterval := time.Second / time.Duration(sendRate)
	var wg sync.WaitGroup
	wg.Add(consumers)
	var dropped, allowed int64
	for j := 0; j < consumers; j++ {
		go func(id int) {
			perConsumerRate := newRateCounter(15, 1)
			for i := 0; i < iters; i++ {
				sampled := perConsumerRate.consume(1)
				ok := b.Count(sampled, true)
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
	t.Logf("TB iters %d allowed %v dropped %v limited %v", iters, allowed, dropped, b.limited)
	assert.True(t, (iters == 100 && consumers == 5))
	assert.True(t, (allowed == 20 && dropped == 480 && b.limited == 230 && b.traced == 20) ||
		(allowed == 19 && dropped == 481 && b.limited == 231 && b.traced == 19) ||
		(allowed == 18 && dropped == 482 && b.limited == 232 && b.traced == 18))
	assert.Equal(t, int64(500), b.requested)
	assert.Equal(t, int64(250), b.sampled)
	assert.Equal(t, int64(500), b.through)
}

func TestOboeRateCounterTime(t *testing.T) {
	b := newRateCounter(5, 2)
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

func startTestUDPListener(t *testing.T, bufs *[][]byte, numbufs int) chan struct{} {
	done := make(chan struct{})
	// set up UDP listener on alternate listen address
	reporterAddr = "127.0.0.1:7832"
	_ := globalReporter()
	assert.IsType(t, &udpReporter{}, globalReporter())

	addr, err := net.ResolveUDPAddr("udp4", reporterAddr)
	assert.NoError(t, err)
	conn, err := net.ListenUDP("udp4", addr)
	assert.NoError(t, err)
	go func(numBufs int) {
		defer conn.Close()
		for i := 0; i < numBufs; i++ {
			buf := make([]byte, 128*1024)
			n, _, err := conn.ReadFromUDP(buf)
			t.Logf("Got UDP buf len %v err %v", n, err)
			if err != nil {
				log.Printf("UDP listener got err, quitting %v", err)
				break
			}
			*bufs = append(*bufs, buf[0:n])
		}
		close(done)
		t.Logf("Closing UDP listener, got %d bufs", numBufs)
	}(numbufs)
	return done
}

func testLayerCount(count int64) interface{} {
	return bson.D{bson.DocElem{Name: testLayer, Value: count}}
}

func init() {
	disableMetrics = true
}

func TestRateSampleRequest(t *testing.T) {
	var bufs [][]byte
	done := startTestUDPListener(t, &bufs, 2)
	sendMetricsMessage(globalReporter())
	<-done
	g.AssertGraph(t, bufs, 2, g.AssertNodeMap{
		{"JMX", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["ProcessName"], initLayer)
			assert.True(t, n.Map[metricsPrefix+"Memory:MemStats.Alloc"].(int64) > 0)
			assert.True(t, n.Map[metricsPrefix+"type=threadcount,name=NumGoroutine"].(int64) > 0)
			// no layer counts yet -- should be missing from message
			assert.NotContains(t, "TraceCount", n.Map)
			assert.NotContains(t, "RequestCount", n.Map)
			assert.NotContains(t, "SampleCount", n.Map)
		}},
		{"JMX", "exit"}: {Edges: g.Edges{{"JMX", "entry"}}},
	})

	traced := int64(0)
	total := 1000
	for i := 0; i < total; i++ {
		if ok, _, _ := shouldTraceRequest(testLayer, ""); ok {
			traced++
		}
	}
	assert.EqualValues(t, traced, 3)
	cl := layerCache.Get(testLayer)
	assert.EqualValues(t, 1000, cl.counter.requested)
	assert.EqualValues(t, 0, cl.counter.through)
	assert.EqualValues(t, traced, cl.counter.traced)
	assert.True(t, cl.counter.sampled > 0)
	assert.True(t, cl.counter.limited > 0)
	sampled := cl.counter.sampled
	limited := cl.counter.limited

	// send UDP message & assert
	bufs = nil
	done = startTestUDPListener(t, &bufs, 2)
	sendMetricsMessage(globalReporter())
	<-done
	g.AssertGraph(t, bufs, 2, g.AssertNodeMap{
		{"JMX", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["ProcessName"], initLayer)
			assert.True(t, n.Map[metricsPrefix+"Memory:MemStats.Alloc"].(int64) > 0)
			assert.True(t, n.Map[metricsPrefix+"type=threadcount,name=NumGoroutine"].(int64) > 0)
			assert.Equal(t, testLayerCount(traced), n.Map["TraceCount"])
			assert.Equal(t, testLayerCount(1000), n.Map["RequestCount"])
			assert.Equal(t, testLayerCount(sampled), n.Map["SampleCount"])
			assert.Equal(t, testLayerCount(traced), n.Map["TraceCount"])
			assert.Equal(t, testLayerCount(limited), n.Map["TokenBucketExhaustionCount"])
		}},
		{"JMX", "exit"}: {Edges: g.Edges{{"JMX", "entry"}}},
	})

	bufs = nil
	done = startTestUDPListener(t, &bufs, 2)
	sendMetricsMessage(globalReporter())
	<-done
	g.AssertGraph(t, bufs, 2, g.AssertNodeMap{
		{"JMX", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["ProcessName"], initLayer)
			assert.True(t, n.Map[metricsPrefix+"Memory:MemStats.Alloc"].(int64) > 0)
			assert.True(t, n.Map[metricsPrefix+"type=threadcount,name=NumGoroutine"].(int64) > 0)
			// all 0s
			assert.EqualValues(t, testLayerCount(0), n.Map["TraceCount"])
			assert.EqualValues(t, testLayerCount(0), n.Map["RequestCount"])
			assert.EqualValues(t, testLayerCount(0), n.Map["SampleCount"])
		}},
		{"JMX", "exit"}: {Edges: g.Edges{{"JMX", "entry"}}},
	})
}

func TestMetrics(t *testing.T) {
	// error sending metrics message: no reporting
	r := SetTestReporter()

	randReader = &errorReader{failOn: map[int]bool{0: true}}
	sendMetricsMessage(r)
	time.Sleep(100 * time.Millisecond)
	r.Close(0)
	assert.Len(t, r.Bufs, 0)

	r = SetTestReporter()
	randReader = &errorReader{failOn: map[int]bool{2: true}}
	sendMetricsMessage(r)
	time.Sleep(100 * time.Millisecond)
	r.Close(0)
	assert.Len(t, r.Bufs, 0)

	randReader = rand.Reader // set back to normal
}

func assertGetNextInterval(t *testing.T, nowTime, expectedDur string) {
	t0, err := time.Parse(time.RFC3339Nano, nowTime)
	assert.NoError(t, err)
	d0 := getNextInterval(t0)
	d0e, err := time.ParseDuration(expectedDur)
	assert.NoError(t, err)
	assert.Equal(t, d0e, d0)
	assert.Equal(t, 0, t0.Add(d0).Second()%counterIntervalSecs)
}

func TestGetNextInterval(t *testing.T) {
	assertGetNextInterval(t, "2016-01-02T15:04:05.888-04:00", "24.112s")
	assertGetNextInterval(t, "2016-01-02T15:04:35.888-04:00", "24.112s")
	assertGetNextInterval(t, "2016-01-02T15:04:00.00-04:00", "30s")
	assertGetNextInterval(t, "2016-08-15T23:31:30.00-00:00", "30s")
	assertGetNextInterval(t, "2016-01-02T15:04:59.999999999-04:00", "1ns")
	assertGetNextInterval(t, "2016-01-07T15:04:29.999999999-00:00", "1ns")
}

func TestSendMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping metrics periodic sender test")
	}
	// full periodic sender test: wait for next interval & report
	r := SetTestReporter(time.Duration(30) * time.Second)
	disableMetrics = false
	go sendInitMessage()
	d0 := getNextInterval(time.Now()) + time.Second
	fmt.Printf("[%v] TestSendMetrics Sleeping for %v\n", time.Now(), d0)
	time.Sleep(d0)
	fmt.Printf("[%v] TestSendMetrics Closing\n", time.Now())
	r.Close(4)
	g.AssertGraph(t, r.Bufs, 4, g.AssertNodeMap{
		{"go", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, 1, n.Map["__Init"])
			assert.Equal(t, initVersion, n.Map["Go.Oboe.Version"])
			assert.NotEmpty(t, n.Map["Oboe.Version"])
			assert.NotEmpty(t, n.Map["Go.Version"])
		}},
		{"go", "exit"}: {Edges: g.Edges{{"go", "entry"}}},
		{metricsLayerName, "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, "go", n.Map["ProcessName"])
			assert.IsType(t, int64(0), n.Map["JMX.type=threadcount,name=NumGoroutine"])
			assert.IsType(t, int64(0), n.Map["JMX.Memory:MemStats.Alloc"])
			assert.True(t, len(n.Map) > 10)
		}},
		{metricsLayerName, "exit"}: {Edges: g.Edges{{metricsLayerName, "entry"}}},
	})
	stopMetrics <- struct{}{}
	disableMetrics = true
}

func TestOboeTracingMode(t *testing.T) {
	_ = SetTestReporter()
	// make oboeSampleRequest think test reporter is not being used for these tests..
	usingTestReporter = false

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "ALWAYS")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settingsCfg.tracing_mode, 1) // C.OBOE_TRACE_ALWAYS

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "tHRoUgh")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settingsCfg.tracing_mode, 2) // C.OBOE_TRACE_THROUGH
	ok, _, _ := oboeSampleRequest("myLayer", "1BJKL")
	assert.True(t, ok)
	ok, _, _ = oboeSampleRequest("myLayer", "")
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "never")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settingsCfg.tracing_mode, 0) // C.OBOE_TRACE_NEVER
	ok, _, _ = oboeSampleRequest("myLayer", "1BJKL")
	assert.False(t, ok)
	ok, _, _ = oboeSampleRequest("myLayer", "")
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settingsCfg.tracing_mode, 1)
}
