// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"errors"
	"testing"

	g "github.com/appneta/go-traceview/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

// Excercise sampling rate logic:
func TestSampleRequest(t *testing.T) {
	_ = SetTestReporter() // set up test reporter
	sampled := 0
	total := 1000
	for i := 0; i < total; i++ {
		if ok, _, _ := shouldTraceRequest(test_layer, ""); ok {
			sampled++
		}
	}
	t.Logf("Sampled %d / %d requests", sampled, total)

	if sampled == 0 {
		t.Errorf("Expected to sample a request.")
	}
}

func TestNullReporter(t *testing.T) {
	reporter = &nullReporter{}
	assert.False(t, reporter.IsOpen())

	// The nullReporter should seem like a regular reporter and not break
	assert.NotPanics(t, func() {
		ctx := NewContext()
		err := ctx.ReportEvent("info", test_layer, "Controller", "test_controller", "Action", "test_action")
		assert.NoError(t, err)
	})

	buf := []byte("xxx")
	cnt, err := reporter.WritePacket(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(buf), cnt)
}

func TestNewReporter(t *testing.T) {
	assert.IsType(t, &udpReporter{}, NewReporter())

	reporterAddr = "127.0.0.1:777831"
	assert.IsType(t, &nullReporter{}, NewReporter())
	reporterAddr = "127.0.0.1:7831"
}

// dependency injection for os.Hostname and net.{ResolveUDPAddr/DialUDP}
type failHostnamer struct{}

func (h failHostnamer) Hostname() (string, error) {
	return "", errors.New("couldn't resolve hostname")
}
func TestCacheHostname(t *testing.T) {
	assert.IsType(t, &udpReporter{}, NewReporter())

	cacheHostname(failHostnamer{})
	assert.IsType(t, &nullReporter{}, NewReporter())
}

func TestReportEvent(t *testing.T) {
	r := SetTestReporter()
	ctx := NewContext()
	assert.Error(t, reportEvent(r, ctx, nil))
	assert.Len(t, r.Bufs, 0) // no reporting

	// mismatched task IDs
	ev := ctx.NewEvent(LabelExit, test_layer)
	assert.Error(t, reportEvent(r, nil, ev))
	assert.Len(t, r.Bufs, 0) // no reporting

	ctx2 := NewContext()
	e2 := ctx2.NewEvent(LabelEntry, "layer2")
	assert.Error(t, reportEvent(r, ctx2, ev))
	assert.Error(t, reportEvent(r, ctx, e2))

	// successful event
	reportEvent(r, ctx, ev)
	assert.Len(t, r.Bufs, 1)
	// re-report: shouldn't work (op IDs the same)
	assert.Error(t, reportEvent(r, ctx, ev))

	g.AssertGraph(t, r.Bufs, 1, map[g.MatchNode]g.AssertNode{
		{"go_test", "exit"}: {},
	})
}
