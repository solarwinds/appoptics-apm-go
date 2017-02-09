// Copyright (C) 2016 Librato, Inc. All rights reserved.

package tv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tracelytics/go-traceview/v1/tv"
	g "github.com/tracelytics/go-traceview/v1/tv/internal/graphtest"
	"github.com/tracelytics/go-traceview/v1/tv/internal/traceview"
	"golang.org/x/net/context"
)

func testProf(ctx context.Context) {
	tv.BeginProfile(ctx, "testProf")
}

func TestBeginProfile(t *testing.T) {
	r := traceview.SetTestReporter()
	ctx := tv.NewContext(context.Background(), tv.NewTrace("testLayer"))
	testProf(ctx)

	r.Close(2)
	g.AssertGraph(t, r.Bufs, 2, g.AssertNodeMap{
		{"testLayer", "entry"}: {},
		{"", "profile_entry"}: {Edges: g.Edges{{"testLayer", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "testProf")
			assert.Equal(t, n.Map["FunctionName"], "github.com/tracelytics/go-traceview/v1/tv_test.testProf")
			assert.Contains(t, n.Map["File"], "/go-traceview/v1/tv/profile_test.go")
		}},
	})
}

func testLayerProf(ctx context.Context) {
	l1, _ := tv.BeginLayer(ctx, "L1")
	p := l1.BeginProfile("testLayerProf")
	p.End()
	l1.End()
	tv.EndTrace(ctx)
}

func TestBeginLayerProfile(t *testing.T) {
	r := traceview.SetTestReporter()
	ctx := tv.NewContext(context.Background(), tv.NewTrace("testLayer"))
	testLayerProf(ctx)

	r.Close(6)
	g.AssertGraph(t, r.Bufs, 6, g.AssertNodeMap{
		{"testLayer", "entry"}: {},
		{"L1", "entry"}:        {Edges: g.Edges{{"testLayer", "entry"}}},
		{"", "profile_entry"}: {Edges: g.Edges{{"L1", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "testLayerProf")
			assert.Equal(t, n.Map["FunctionName"], "github.com/tracelytics/go-traceview/v1/tv_test.testLayerProf")
			assert.Contains(t, n.Map["File"], "/go-traceview/v1/tv/profile_test.go")
		}},
		{"", "profile_exit"}: {Edges: g.Edges{{"", "profile_entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "testLayerProf")
		}},
		{"L1", "exit"}:        {Edges: g.Edges{{"", "profile_exit"}, {"L1", "entry"}}},
		{"testLayer", "exit"}: {Edges: g.Edges{{"L1", "exit"}, {"testLayer", "entry"}}},
	})

}

// ensure above tests run smoothly with no events reported when a context has no trace
func TestNoTraceBeginProfile(t *testing.T) {
	r := traceview.SetTestReporter()
	ctx := context.Background()
	testProf(ctx)
	r.Close(0)
	assert.Len(t, r.Bufs, 0)
}
func TestTraceErrorBeginProfile(t *testing.T) {
	// simulate reporter error on second event: prevents Layer from being reported
	r := traceview.SetTestReporter()
	r.ErrorEvents = map[int]bool{1: true}
	testProf(tv.NewContext(context.Background(), tv.NewTrace("testLayer")))
	r.Close(1)
	g.AssertGraph(t, r.Bufs, 1, g.AssertNodeMap{
		{"testLayer", "entry"}: {},
	})
}

func TestNoTraceBeginLayerProfile(t *testing.T) {
	r := traceview.SetTestReporter()
	ctx := context.Background()
	testLayerProf(ctx)
	r.Close(0)
	assert.Len(t, r.Bufs, 0)
}
func TestTraceErrorBeginLayerProfile(t *testing.T) {
	// simulate reporter error on second event: prevents nested Layer & Profile spans
	r := traceview.SetTestReporter()
	r.ErrorEvents = map[int]bool{1: true}
	testLayerProf(tv.NewContext(context.Background(), tv.NewTrace("testLayer")))
	r.Close(2)
	g.AssertGraph(t, r.Bufs, 2, g.AssertNodeMap{
		{"testLayer", "entry"}: {},
		{"testLayer", "exit"}:  {Edges: g.Edges{{"testLayer", "entry"}}},
	})
}
