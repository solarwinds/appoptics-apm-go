// +build traceview

// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"os"
	"testing"

	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

func TestInitMessage(t *testing.T) {
	r := SetTestReporter()

	sendInitMessage()

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go", "entry"}: {g.OutEdges{}, func(n g.Node) {
			assert.Equal(t, 1, n.Map["__Init"])
			assert.Equal(t, initVersion, n.Map["Go.Oboe.Version"])
			assert.NotEmpty(t, n.Map["Oboe.Version"])
			assert.NotEmpty(t, n.Map["Go.Version"])
		}},
		{"go", "exit"}: {g.OutEdges{{"go", "entry"}}, nil},
	})
}

func TestOboeTracingMode(t *testing.T) {
	_ = SetTestReporter()
	// make oboeSampleRequest think test reporter is not being used for these tests..
	usingTestReporter = false

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "ALWAYS")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 1) // C.OBOE_TRACE_ALWAYS

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "tHRoUgh")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 2) // C.OBOE_TRACE_THROUGH
	ok, _, _ := oboeSampleRequest("myLayer", "1BXXX")
	assert.True(t, ok)
	ok, _, _ = oboeSampleRequest("myLayer", "")
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "never")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 0) // C.OBOE_TRACE_NEVER
	ok, _, _ = oboeSampleRequest("myLayer", "1BXXX")
	assert.False(t, ok)
	ok, _, _ = oboeSampleRequest("myLayer", "")
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 1)
}
