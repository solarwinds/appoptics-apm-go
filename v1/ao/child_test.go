// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao_test

import (
	"testing"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/stretchr/testify/assert"
	"context"
)

func childExample(ctx context.Context) {
	// create a new trace, and a context to carry it around
	l1, _ := ao.BeginSpan(ctx, "L1")
	l2 := l1.BeginSpan("DBx", "Query", "SELECT * FROM tbl")
	time.Sleep(20 * time.Millisecond)
	l2.End()
	l1.End()

	// test attempting to start a child from a span that has ended
	// currently we don't allow this, so nothing should be reported
	l3 := l1.BeginSpan("invalidSpan", "notReported", true)
	l3.End()

	// test attempting to start a profile from a span that has ended
	// similarly we don't allow this, so nothing should be reported
	p1 := l1.BeginProfile("f2")
	p1.End()

	// end the trace
	ao.EndTrace(ctx)
}

func childExampleCtx(ctx context.Context) {
	// create a new trace, and a context to carry it around
	_, ctxL1 := ao.BeginSpan(ctx, "L1")
	_, ctxL2 := ao.BeginSpan(ctxL1, "DBx", "Query", "SELECT * FROM tbl")
	time.Sleep(20 * time.Millisecond)
	ao.End(ctxL2)
	ao.End(ctxL1)

	// test attempting to start a child from a span that has ended
	// currently we don't allow this, so nothing should be reported
	_, ctxL3 := ao.BeginSpan(ctxL1, "invalidSpan", "notReported", true)
	ao.End(ctxL3)

	// test attempting to start a profile from a span that has ended
	// similarly we don't allow this, so nothing should be reported
	p1 := ao.BeginProfile(ctxL1, "f2")
	p1.End()

	// end the trace
	ao.EndTrace(ctx)
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
	r := reporter.SetTestReporter() // enable test reporter
	ctx := ao.NewContext(context.Background(), ao.NewTrace("childExample"))
	childExample(ctx) // generate events
	r.Close(6)
	assertTraceChild(t, r.EventBufs)
}

func TestTraceChildCtx(t *testing.T) {
	r := reporter.SetTestReporter() // enable test reporter
	ctx := ao.NewContext(context.Background(), ao.NewTrace("childExample"))
	childExampleCtx(ctx) // generate events
	r.Close(6)
	assertTraceChild(t, r.EventBufs)
}

func TestNoTraceChild(t *testing.T) {
	r := reporter.SetTestReporter()
	ctx := context.Background()
	childExample(ctx)
	assert.Len(t, r.EventBufs, 0)

	r = reporter.SetTestReporter()
	ctx = context.Background()
	childExampleCtx(ctx)
	assert.Len(t, r.EventBufs, 0)
}
