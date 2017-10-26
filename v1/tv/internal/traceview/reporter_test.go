// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"errors"
	"os"
	"testing"
	"time"

	g "github.com/librato/go-traceview/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

const (
	collectorAddress = "127.0.0.1"
	serviceKey       = "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"
)

// this runs before init()
var _ = func() (_ struct{}) {
	os.Setenv("APPOPTICS_SERVICE_KEY", serviceKey)
	os.Setenv("APPOPTICS_COLLECTOR", collectorAddress)
	return
}()

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
	assert.Len(t, r.Bufs, 0) // no reporting

	// mismatched task IDs
	ev, err := ctx.newEvent(LabelExit, testLayer)
	assert.NoError(t, err)
	assert.Error(t, r.reportEvent(nil, ev))
	assert.Len(t, r.Bufs, 0) // no reporting

	ctx2 := newTestContext(t)
	e2, err := ctx2.newEvent(LabelEntry, "layer2")
	assert.NoError(t, err)
	assert.Error(t, r.reportEvent(ctx2, ev))
	assert.Error(t, r.reportEvent(ctx, e2))

	// successful event
	assert.NoError(t, r.reportEvent(ctx, ev))
	r.Close(1)
	assert.Len(t, r.Bufs, 1)

	// re-report: shouldn't work (op IDs the same, reporter closed)
	assert.Error(t, r.reportEvent(ctx, ev))

	g.AssertGraph(t, r.Bufs, 1, g.AssertNodeMap{
		{"go_test", "exit"}: {},
	})
}

// test behavior of the TestReporter
func TestTestReporter(t *testing.T) {
	r := SetTestReporter()
	r.Close(1) // wait on event that will never be reported: causes timeout
	assert.Len(t, r.Bufs, 0)

	r = SetTestReporter()
	go func() { // simulate late event
		time.Sleep(100 * time.Millisecond)
		ctx := newTestContext(t)
		ev, err := ctx.newEvent(LabelExit, testLayer)
		assert.NoError(t, err)
		assert.NoError(t, r.reportEvent(ctx, ev))
	}()
	r.Close(1) // wait on late event -- blocks until timeout or event received
	assert.Len(t, r.Bufs, 1)

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
	assert.Equal(t, nil, nullR.reportEvent(nil, nil))
	assert.Equal(t, nil, nullR.reportSpan(nil))
}

// ========================= UDP Reporter =============================

func assertUDPMode(t *testing.T) {
	// for UDP mode run test like this:
	// APPOPTICS_REPORTER=udp go test -v

	if os.Getenv("APPOPTICS_REPORTER") != "udp" {
		t.Skip("not running in UDP mode, skipping.")
	}
}

func TestUDPReporter(t *testing.T) {
	assertUDPMode(t)
	// TODO implement
}

// ========================= GRPC Reporter =============================

func assertSSLMode(t *testing.T) {
	if os.Getenv("APPOPTICS_REPORTER") == "udp" {
		t.Skip("not running in SSL mode, skipping.")
	}
}

func TestGRPCReporter(t *testing.T) {
	assertSSLMode(t)
	assert.IsType(t, &grpcReporter{}, thisReporter)

	r := thisReporter.(*grpcReporter)

	assert.Equal(t, collectorAddress, r.eventConnection.address)
	assert.Equal(t, collectorAddress, r.metricConnection.address)

	assert.Equal(t, serviceKey, r.eventConnection.serviceKey)
	assert.Equal(t, serviceKey, r.metricConnection.serviceKey)

	assert.Equal(t, grpcMetricIntervalDefault, r.collectMetricInterval)
	assert.Equal(t, grpcGetSettingsIntervalDefault, r.getSettingsInterval)
	assert.Equal(t, grpcSettingsTimeoutCheckIntervalDefault, r.settingsTimeoutCheckInterval)
}
