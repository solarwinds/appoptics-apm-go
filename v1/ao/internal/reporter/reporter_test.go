// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"strconv"

	"bytes"

	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/agent"
	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	pb "github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

const (
	serviceKey = "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go"
)

// this runs before init()
var _ = func() (_ struct{}) {
	os.Setenv("APPOPTICS_SERVICE_KEY", serviceKey)
	os.Setenv("APPOPTICS_REPORTER", "none")
	// Call agent.Init() after changing any environment variables for testing.
	agent.Init()
	return
}()

func init() {
	periodicTasksDisabled = true
}

// dependency injection for os.Hostname and net.{ResolveUDPAddr/DialUDP}
type failHostnamer struct{}

func (h failHostnamer) Hostname() (string, error) {
	return "", errors.New("couldn't resolve hostname")
}
func TestCacheHostname(t *testing.T) {
	h, _ := os.Hostname()
	assert.Equal(t, h, cachedHostname)
	assert.Equal(t, false, reportingDisabled)
	t.Logf("Forcing hostname error: 'Unable to get hostname' log message expected")
	cacheHostname(failHostnamer{})
	assert.Equal(t, "", cachedHostname)
	assert.Equal(t, true, reportingDisabled)
}

// ========================= Test Reporter =============================

func TestReportEvent(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	assert.Error(t, r.reportEvent(ctx, nil))
	assert.Len(t, r.EventBufs, 0) // no reporting

	// mismatched task IDs
	ev, err := ctx.newEvent(LabelExit, testLayer)
	assert.NoError(t, err)
	assert.Error(t, r.reportEvent(nil, ev))
	assert.Len(t, r.EventBufs, 0) // no reporting

	ctx2 := newTestContext(t)
	e2, err := ctx2.newEvent(LabelEntry, "layer2")
	assert.NoError(t, err)
	assert.Error(t, r.reportEvent(ctx2, ev))
	assert.Error(t, r.reportEvent(ctx, e2))

	// successful event
	assert.NoError(t, r.reportEvent(ctx, ev))
	r.Close(1)
	assert.Len(t, r.EventBufs, 1)

	// re-report: shouldn't work (op IDs the same, reporter closed)
	assert.Error(t, r.reportEvent(ctx, ev))

	g.AssertGraph(t, r.EventBufs, 1, g.AssertNodeMap{
		{"go_test", "exit"}: {},
	})
}

func TestReportMetric(t *testing.T) {
	r := SetTestReporter()
	spanMsg := &HTTPSpanMessage{
		BaseSpanMessage: BaseSpanMessage{
			Duration: time.Second,
			HasError: false,
		},
		Transaction: "tname",
		Path:        "/path/to/url",
		Status:      203,
		Method:      "HEAD",
	}
	err := ReportSpan(spanMsg)
	assert.NoError(t, err)
	r.Close(1)
	assert.Len(t, r.SpanMessages, 1)
	sp, ok := r.SpanMessages[0].(*HTTPSpanMessage)
	require.True(t, ok)
	require.NotNil(t, sp)
	assert.True(t, reflect.DeepEqual(spanMsg, sp))
}

// test behavior of the TestReporter
func TestTestReporter(t *testing.T) {
	r := SetTestReporter()
	r.Close(0) // wait on event that will never be reported: causes timeout
	assert.Len(t, r.EventBufs, 0)

	r = SetTestReporter()
	go func() { // simulate late event
		time.Sleep(100 * time.Millisecond)
		ctx := newTestContext(t)
		ev, err := ctx.newEvent(LabelExit, testLayer)
		assert.NoError(t, err)
		assert.NoError(t, r.reportEvent(ctx, ev))
	}()
	r.Close(1) // wait on late event -- blocks until timeout or event received
	assert.Len(t, r.EventBufs, 1)

	// send an event after calling Close -- should panic
	assert.Panics(t, func() {
		ctx := newTestContext(t)
		ev, err := ctx.newEvent(LabelExit, testLayer)
		assert.NoError(t, err)
		assert.NoError(t, r.reportEvent(ctx, ev))
	})
}

// ========================= NULL Reporter =============================

func TestNullReporter(t *testing.T) {
	nullR := &nullReporter{}
	assert.NoError(t, nullR.reportEvent(nil, nil))
	assert.NoError(t, nullR.reportStatus(nil, nil))
	assert.NoError(t, nullR.reportSpan(nil))
}

// ========================= UDP Reporter =============================
func startTestUDPListener(t *testing.T, bufs *[][]byte, numbufs int) chan struct{} {
	done := make(chan struct{})
	assert.IsType(t, &udpReporter{}, globalReporter)

	addr, err := net.ResolveUDPAddr("udp4", os.Getenv("APPOPTICS_COLLECTOR_UDP"))
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

func assertUDPMode(t *testing.T) {
	// for UDP mode run test like this:
	// APPOPTICS_REPORTER=udp go test -v

	if os.Getenv("APPOPTICS_REPORTER") != "udp" {
		t.Skip("not running in UDP mode, skipping.")
	}
}

func TestUDPReporter(t *testing.T) {
	assertUDPMode(t)
	assert.IsType(t, &udpReporter{}, globalReporter)

	r := globalReporter.(*udpReporter)
	ctx := newTestContext(t)
	ev1, _ := ctx.newEvent(LabelInfo, testLayer)
	ev2, _ := ctx.newEvent(LabelInfo, testLayer)

	var bufs [][]byte
	startTestUDPListener(t, &bufs, 2)

	assert.Error(t, r.reportEvent(nil, nil))
	assert.Error(t, r.reportEvent(ctx, nil))
	assert.NoError(t, r.reportEvent(ctx, ev1))

	assert.Error(t, r.reportStatus(nil, nil))
	assert.Error(t, r.reportStatus(ctx, nil))
	assert.NoError(t, r.reportStatus(ctx, ev2))
}

// ========================= GRPC Reporter =============================

func assertSSLMode(t *testing.T) {
	if os.Getenv("APPOPTICS_REPORTER") == "udp" {
		t.Skip("not running in SSL mode, skipping.")
	}
}

func TestGRPCReporter(t *testing.T) {
	// start test gRPC server
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	agent.Init()
	addr := "localhost:4567"
	server := StartTestGRPCServer(t, addr)
	time.Sleep(100 * time.Millisecond)

	// set gRPC reporter
	reportingDisabled = false
	os.Setenv("APPOPTICS_COLLECTOR", addr)
	os.Setenv("APPOPTICS_TRUSTEDPATH", testCertFile)
	agent.Init()
	oldReporter := globalReporter
	setGlobalReporter("ssl")

	require.IsType(t, &grpcReporter{}, globalReporter)

	r := globalReporter.(*grpcReporter)
	ctx := newTestContext(t)
	ev1, err := ctx.newEvent(LabelInfo, "layer1")
	assert.NoError(t, err)
	ev2, err := ctx.newEvent(LabelInfo, "layer2")
	assert.NoError(t, err)

	assert.Error(t, r.reportEvent(nil, nil))
	assert.Error(t, r.reportEvent(ctx, nil))
	assert.NoError(t, r.reportEvent(ctx, ev1))

	assert.Error(t, r.reportStatus(nil, nil))
	assert.Error(t, r.reportStatus(ctx, nil))
	time.Sleep(time.Millisecond * 100)
	assert.NoError(t, r.reportStatus(ctx, ev2))

	assert.Equal(t, addr, r.eventConnection.address)
	assert.Equal(t, addr, r.metricConnection.address)

	assert.Equal(t, serviceKey, r.eventConnection.serviceKey)
	assert.Equal(t, serviceKey, r.metricConnection.serviceKey)

	assert.Equal(t, grpcMetricIntervalDefault, r.collectMetricInterval)
	assert.Equal(t, grpcGetSettingsIntervalDefault, r.getSettingsInterval)
	assert.Equal(t, grpcSettingsTimeoutCheckIntervalDefault, r.settingsTimeoutCheckInterval)

	time.Sleep(150 * time.Millisecond)

	// stop test reporter
	server.Stop()
	globalReporter = oldReporter

	// assert data received
	require.Len(t, server.events, 1)
	assert.Equal(t, server.events[0].Encoding, pb.EncodingType_BSON)
	require.Len(t, server.events[0].Messages, 1)

	require.Len(t, server.status, 3)
	assert.Equal(t, server.status[0].Encoding, pb.EncodingType_BSON)
	require.Len(t, server.status[0].Messages, 1)
	require.Len(t, server.status[1].Messages, 1)
	require.Len(t, server.status[2].Messages, 1)

	dec1, dec2, dec3, dec4 := bson.M{}, bson.M{}, bson.M{}, bson.M{}
	err = bson.Unmarshal(server.events[0].Messages[0], &dec1)
	require.NoError(t, err)
	err = bson.Unmarshal(server.status[0].Messages[0], &dec2)
	require.NoError(t, err)
	err = bson.Unmarshal(server.status[1].Messages[0], &dec3)
	require.NoError(t, err)
	err = bson.Unmarshal(server.status[2].Messages[0], &dec4)
	require.NoError(t, err)

	assert.Equal(t, dec1["Layer"], "layer1")
	assert.Equal(t, dec4["Layer"], "layer2")

	// assert ConnectionInit messages
	assert.Equal(t, dec2["ConnectionInit"], true)
	assert.Equal(t, dec3["ConnectionInit"], true)
	assert.Equal(t, dec2["Hostname"], cachedHostname)
	assert.Equal(t, dec3["Hostname"], cachedHostname)
	assert.Equal(t, dec2["Distro"], getDistro())
	assert.Equal(t, dec3["Distro"], getDistro())
}

func TestInterruptedGRPCReporter(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	// start test gRPC server
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "info")
	agent.Init()
	addr := "localhost:4567"
	server := StartTestGRPCServer(t, addr)
	time.Sleep(100 * time.Millisecond)

	// set gRPC reporter
	reportingDisabled = false
	os.Setenv("APPOPTICS_COLLECTOR", addr)
	os.Setenv("APPOPTICS_TRUSTEDPATH", testCertFile)
	agent.Init()
	oldReporter := globalReporter
	setGlobalReporter("ssl")

	require.IsType(t, &grpcReporter{}, globalReporter)

	r := globalReporter.(*grpcReporter)
	var bsonArr []bson.M

	for i := 1; i <= 10; i++ {
		ctx := newTestContext(t)
		ev1, _ := ctx.newEvent(LabelInfo, "layer-"+strconv.Itoa(i))
		assert.NoError(t, r.reportEvent(ctx, ev1))
		time.Sleep(time.Millisecond * 50)
	}
	time.Sleep(time.Second * 10)
	// stop test reporter
	server.Stop()

	for i := 0; i < len(server.events); i++ {
		for j := 0; j < len(server.events[i].Messages); j++ {
			bs := bson.M{}
			bson.Unmarshal(server.events[i].Messages[j], &bs)
			bsonArr = append(bsonArr, bs)
		}
	}

	for i := 11; i <= 20; i++ {
		ctx := newTestContext(t)
		ev1, _ := ctx.newEvent(LabelInfo, "layer-"+strconv.Itoa(i))
		assert.NoError(t, r.reportEvent(ctx, ev1))
		time.Sleep(time.Millisecond * 50)
	}
	time.Sleep(time.Second * 60)

	server = StartTestGRPCServer(t, addr)
	time.Sleep(time.Second * 10)

	for i := 21; i <= 30; i++ {
		ctx := newTestContext(t)
		ev1, _ := ctx.newEvent(LabelInfo, "layer-"+strconv.Itoa(i))
		assert.NoError(t, r.reportEvent(ctx, ev1))
		time.Sleep(time.Millisecond * 50)
	}

	time.Sleep(time.Second * 30)
	globalReporter = oldReporter
	server.Stop()

	s := buf.String()
	assert.True(t, strings.Contains(s, "Error calling PostEvents"))
	assert.True(t, strings.Contains(s, "Error recovered in PostEvents"))

	for i := 0; i < len(server.events); i++ {
		for j := 0; j < len(server.events[i].Messages); j++ {
			bs := bson.M{}
			bson.Unmarshal(server.events[i].Messages[j], &bs)
			bsonArr = append(bsonArr, bs)
		}
	}
	assert.Equal(t, 30, len(bsonArr))

}

func TestRedirect(t *testing.T) {
	// start test gRPC server
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	agent.Init()
	addr := "localhost:4567"
	server := StartTestGRPCServer(t, addr)
	time.Sleep(100 * time.Millisecond)

	// set gRPC reporter
	reportingDisabled = false
	os.Setenv("APPOPTICS_COLLECTOR", addr)
	os.Setenv("APPOPTICS_TRUSTEDPATH", testCertFile)
	agent.Init()
	oldReporter := globalReporter
	setGlobalReporter("ssl")

	require.IsType(t, &grpcReporter{}, globalReporter)

	r := globalReporter.(*grpcReporter)
	var bsonArr []bson.M

	for i := 1; i <= 10; i++ {
		ctx := newTestContext(t)
		ev1, _ := ctx.newEvent(LabelInfo, "layer-"+strconv.Itoa(i))
		assert.NoError(t, r.reportEvent(ctx, ev1))
		time.Sleep(time.Millisecond * 50)
	}
	time.Sleep(time.Second * 10)
	// stop test reporter
	server.Stop()

	for i := 0; i < len(server.events); i++ {
		for j := 0; j < len(server.events[i].Messages); j++ {
			bs := bson.M{}
			bson.Unmarshal(server.events[i].Messages[j], &bs)
			bsonArr = append(bsonArr, bs)
		}
	}

	addr2 := "localhost:4568"
	server = StartTestGRPCServer(t, addr2)
	time.Sleep(time.Second * 10)
	// Call redirect directly
	r.redirectTo(r.eventConnection, addr2)

	for i := 11; i <= 20; i++ {
		ctx := newTestContext(t)
		ev1, _ := ctx.newEvent(LabelInfo, "layer-"+strconv.Itoa(i))
		assert.NoError(t, r.reportEvent(ctx, ev1))
		time.Sleep(time.Millisecond * 50)
	}
	time.Sleep(time.Second * 10)

	globalReporter = oldReporter
	server.Stop()

	for i := 0; i < len(server.events); i++ {
		for j := 0; j < len(server.events[i].Messages); j++ {
			bs := bson.M{}
			bson.Unmarshal(server.events[i].Messages[j], &bs)
			bsonArr = append(bsonArr, bs)
		}
	}
	assert.Equal(t, 20, len(bsonArr))

}

func TestShutdownGRPCReporter(t *testing.T) {
	// var buf bytes.Buffer
	// log.SetOutput(&buf)
	// defer func() {
	// 	log.SetOutput(os.Stderr)
	// }()

	// start test gRPC server
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	addr := "localhost:4567"
	server := StartTestGRPCServer(t, addr)
	time.Sleep(100 * time.Millisecond)

	// set gRPC reporter
	reportingDisabled = false
	os.Setenv("APPOPTICS_COLLECTOR", addr)
	os.Setenv("APPOPTICS_TRUSTEDPATH", testCertFile)
	agent.Init()
	oldReporter := globalReporter
	// numGo := runtime.NumGoroutine()
	setGlobalReporter("ssl")

	require.IsType(t, &grpcReporter{}, globalReporter)
	time.Sleep(time.Second * 5)

	r := globalReporter.(*grpcReporter)
	r.Shutdown()
	time.Sleep(time.Second * 5)
	assert.Equal(t, true, r.Closed())

	// // Print current goroutines stack
	// buf := make([]byte, 1<<16)
	// runtime.Stack(buf, true)
	// fmt.Printf("%s", buf)

	e := r.Shutdown()
	assert.NotEqual(t, nil, e)

	// stop test reporter
	server.Stop()
	globalReporter = oldReporter
	// fmt.Println(buf)
}

func TestInvalidKey(t *testing.T) {
	var buf SafeBuffer
	var writers []io.Writer

	writers = append(writers, &buf)
	writers = append(writers, os.Stderr)

	log.SetOutput(io.MultiWriter(writers...))

	defer func() {
		log.SetOutput(os.Stderr)
	}()

	invalidKey := "invalidf6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	oldKey := os.Getenv("APPOPTICS_SERVICE_KEY")
	os.Setenv("APPOPTICS_SERVICE_KEY", invalidKey)
	addr := "localhost:4567"
	os.Setenv("APPOPTICS_COLLECTOR", addr)
	os.Setenv("APPOPTICS_TRUSTEDPATH", testCertFile)
	reportingDisabled = false

	// start test gRPC server
	server := StartTestGRPCServer(t, addr)
	time.Sleep(100 * time.Millisecond)

	// set gRPC reporter
	agent.Init()
	oldReporter := globalReporter
	// numGo := runtime.NumGoroutine()
	setGlobalReporter("ssl")

	require.IsType(t, &grpcReporter{}, globalReporter)
	time.Sleep(time.Second * 5)

	r := globalReporter.(*grpcReporter)
	ctx := newTestContext(t)
	ev1, _ := ctx.newEvent(LabelInfo, "hello-from-invalid-key")
	assert.NoError(t, r.reportEvent(ctx, ev1))
	fmt.Println("Sent a message at: ", time.Now())

	time.Sleep(5 * time.Second)

	// The agent reporter should be closed due to received INVALID_API_KEY from the collector
	assert.Equal(t, true, r.Closed())
	fmt.Println("Shutdown the agent by the user.")
	e := r.Shutdown()
	assert.NotEqual(t, nil, e)

	// Tear down everything.
	server.Stop()
	globalReporter = oldReporter
	os.Setenv("APPOPTICS_SERVICE_KEY", oldKey)

	patterns := []string{
		"Server responded: Invalid API key. Reporter is closing",
		"Shutting down the gRPC reporter",
		// "periodicTasks goroutine exiting",
		"eventSender goroutine exiting",
		"spanMessageAggregator goroutine exiting",
		"statusSender goroutine exiting",
		"eventRetrySender goroutine exiting",
	}
	for _, ptn := range patterns {
		assert.True(t, strings.Contains(buf.String(), ptn))
	}

}

// SafeBuffer is goroutine-safe buffer. It is for internal test use only.
type SafeBuffer struct {
	buf bytes.Buffer
	sync.Mutex
}

func (b *SafeBuffer) Read(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.buf.Read(p)
}

func (b *SafeBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.buf.Write(p)
}

func (b *SafeBuffer) String() string {
	b.Lock()
	defer b.Unlock()
	return b.buf.String()
}
