// Copyright (C) 2016 Librato, Inc. All rights reserved.

package tv_test

import (
	"testing"

	"github.com/librato/go-traceview/v1/tv"
	g "github.com/librato/go-traceview/v1/tv/internal/graphtest"
	"github.com/librato/go-traceview/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func testProf(ctx context.Context) {
	tv.BeginProfile(ctx, "testProf")
}

func TestBeginProfile(t *testing.T) {
	r := traceview.SetTestReporter(true)
	ctx := tv.NewContext(context.Background(), tv.NewTrace("testSpan"))
	testProf(ctx)

	r.Close(2)
	g.AssertGraph(t, r.Bufs, 2, g.AssertNodeMap{
		{"testSpan", "entry"}: {},
		{"", "profile_entry"}: {Edges: g.Edges{{"testSpan", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "testProf")
			assert.Equal(t, n.Map["FunctionName"], "github.com/librato/go-traceview/v1/tv_test.testProf")
			assert.Contains(t, n.Map["File"], "/go-traceview/v1/tv/profile_test.go")
		}},
	})
}

func testSpanProf(ctx context.Context) {
	l1, _ := tv.BeginSpan(ctx, "L1")
	p := l1.BeginProfile("testSpanProf")
	p.End()
	l1.End()
	tv.EndTrace(ctx)
}

func TestBeginSpanProfile(t *testing.T) {
	r := traceview.SetTestReporter(true)
	ctx := tv.NewContext(context.Background(), tv.NewTrace("testSpan"))
	testSpanProf(ctx)

	r.Close(6)
	g.AssertGraph(t, r.Bufs, 6, g.AssertNodeMap{
		{"testSpan", "entry"}: {},
		{"L1", "entry"}:       {Edges: g.Edges{{"testSpan", "entry"}}},
		{"", "profile_entry"}: {Edges: g.Edges{{"L1", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "testSpanProf")
			assert.Equal(t, n.Map["FunctionName"], "github.com/librato/go-traceview/v1/tv_test.testSpanProf")
			assert.Contains(t, n.Map["File"], "/go-traceview/v1/tv/profile_test.go")
		}},
		{"", "profile_exit"}: {Edges: g.Edges{{"", "profile_entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "testSpanProf")
		}},
		{"L1", "exit"}:       {Edges: g.Edges{{"", "profile_exit"}, {"L1", "entry"}}},
		{"testSpan", "exit"}: {Edges: g.Edges{{"L1", "exit"}, {"testSpan", "entry"}}},
	})

}

// ensure above tests run smoothly with no events reported when a context has no trace
func TestNoTraceBeginProfile(t *testing.T) {
	r := traceview.SetTestReporter(true)
	ctx := context.Background()
	testProf(ctx)
	r.Close(0)
	assert.Len(t, r.EventBufs, 0)
}
func TestTraceErrorBeginProfile(t *testing.T) {
	// simulate reporter error on second event: prevents Span from being reported
	r := traceview.SetTestReporter(true)
	r.ErrorEvents = map[int]bool{1: true}
	testProf(tv.NewContext(context.Background(), tv.NewTrace("testSpan")))
	r.Close(1)
	g.AssertGraph(t, r.Bufs, 1, g.AssertNodeMap{
		{"testSpan", "entry"}: {},
	})
}

func TestNoTraceBeginSpanProfile(t *testing.T) {
	r := traceview.SetTestReporter(true)
	ctx := context.Background()
	testSpanProf(ctx)
	r.Close(0)
	assert.Len(t, r.EventBufs, 0)
}
func TestTraceErrorBeginSpanProfile(t *testing.T) {
	// simulate reporter error on second event: prevents nested Span & Profile spans
	r := traceview.SetTestReporter(true)
	r.ErrorEvents = map[int]bool{1: true}
	testSpanProf(tv.NewContext(context.Background(), tv.NewTrace("testSpan")))
	r.Close(2)
	g.AssertGraph(t, r.Bufs, 2, g.AssertNodeMap{
		{"testSpan", "entry"}: {},
		{"testSpan", "exit"}:  {Edges: g.Edges{{"testSpan", "entry"}}},
	})
}
