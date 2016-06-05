// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"testing"
	"time"

	"github.com/appneta/go-appneta/v1/tv"
	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func childExample(ctx context.Context) {
	// create a new trace, and a context to carry it around
	l1, _ := tv.BeginLayer(ctx, "L1")
	l2 := l1.BeginLayer("DBx", "Query", "SELECT * FROM tbl")
	time.Sleep(20 * time.Millisecond)
	l2.End()
	l1.End()

	// test attempting to start a child from a layer that has ended
	// currently we don't allow this, so nothing should be reported
	l3 := l1.BeginLayer("layer", "notReported", true)
	l3.End()

	// test attempting to start a profile from a layer that has ended
	// similarly we don't allow this, so nothing should be reported
	p1 := l1.BeginProfile("f2")
	p1.End()

	// end the trace
	tv.EndTrace(ctx)
}

func childExampleCtx(ctx context.Context) {
	// create a new trace, and a context to carry it around
	_, ctxL1 := tv.BeginLayer(ctx, "L1")
	_, ctxL2 := tv.BeginLayer(ctxL1, "DBx", "Query", "SELECT * FROM tbl")
	time.Sleep(20 * time.Millisecond)
	tv.End(ctxL2)
	tv.End(ctxL1)

	// test attempting to start a child from a layer that has ended
	// currently we don't allow this, so nothing should be reported
	_, ctxL3 := tv.BeginLayer(ctxL1, "layer", "notReported", true)
	tv.End(ctxL3)

	// test attempting to start a profile from a layer that has ended
	// similarly we don't allow this, so nothing should be reported
	p1 := tv.BeginProfile(ctxL1, "f2")
	p1.End()

	// end the trace
	tv.EndTrace(ctx)
}

func assertTraceChild(t *testing.T, bufs [][]byte) {
	// validate events reported
	g.AssertGraph(t, bufs, 6, g.AssertNodeMap{
		{"childExample", "entry"}: {},
		{"L1", "entry"}:           {Edges: g.Edges{{"childExample", "entry"}}},
		{"DBx", "entry"}:          {Edges: g.Edges{{"L1", "entry"}}},
		{"DBx", "exit"}:           {Edges: g.Edges{{"DBx", "entry"}}},
		{"L1", "exit"}:            {Edges: g.Edges{{"DBx", "exit"}, {"L1", "entry"}}},
		{"childExample", "exit"}:  {Edges: g.Edges{{"L1", "exit"}, {"childExample", "entry"}}},
	})
}

func TestTraceChild(t *testing.T) {
	r := traceview.SetTestReporter() // enable test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("childExample"))
	childExample(ctx) // generate events
	r.Close(6)
	assertTraceChild(t, r.Bufs)
}

func TestTraceChildCtx(t *testing.T) {
	r := traceview.SetTestReporter() // enable test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("childExample"))
	childExampleCtx(ctx) // generate events
	r.Close(6)
	assertTraceChild(t, r.Bufs)
}

func TestNoTraceChild(t *testing.T) {
	r := traceview.SetTestReporter()
	ctx := context.Background()
	childExample(ctx)
	assert.Len(t, r.Bufs, 0)

	r = traceview.SetTestReporter()
	ctx = context.Background()
	childExampleCtx(ctx)
	assert.Len(t, r.Bufs, 0)
}
