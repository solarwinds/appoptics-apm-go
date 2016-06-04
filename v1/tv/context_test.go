// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import (
	"reflect"
	"testing"

	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestContext(t *testing.T) {
	r := traceview.SetTestReporter()

	ctx := context.Background()
	assert.Empty(t, MetadataString(ctx))
	tr := NewTrace("test").(*tvTrace)
	xt := tr.tvCtx.MetadataString()

	ctx2 := context.WithValue(ctx, "t", tr)
	assert.Equal(t, ctx2.Value("t"), tr)
	assert.Equal(t, ctx2.Value("t").(*tvTrace).tvCtx.MetadataString(), xt)

	ctxx := tr.tvCtx.Copy()
	lbl := layerLabeler{"L1"}
	tr2 := &tvTrace{layerSpan: layerSpan{span: span{tvCtx: ctxx, labeler: lbl}}}
	ctx3 := context.WithValue(ctx2, "t", tr2)
	assert.Equal(t, ctx3.Value("t"), tr2)

	ctxx2 := tr2.tvCtx.Copy()
	tr3 := &tvTrace{layerSpan: layerSpan{span: span{tvCtx: ctxx2}}}
	ctx4 := context.WithValue(ctx3, "t", tr3)
	assert.Equal(t, ctx4.Value("t"), tr3)

	g.AssertGraph(t, r.Bufs, 1, g.AssertNodeMap{{"test", "entry"}: {}})
}

func TestTraceFromContext(t *testing.T) {
	r := traceview.SetTestReporter()
	tr := NewTrace("TestTFC")
	ctx := NewContext(context.Background(), tr)
	trFC := TraceFromContext(ctx)
	assert.Equal(t, tr.ExitMetadata(), trFC.ExitMetadata())
	assert.Len(t, tr.ExitMetadata(), 58)

	trN := TraceFromContext(context.Background()) // no trace bound to this ctx
	assert.Len(t, trN.ExitMetadata(), 0)

	g.AssertGraph(t, r.Bufs, 1, g.AssertNodeMap{{"TestTFC", "entry"}: {}})
}

func TestNullSpan(t *testing.T) {
	// enable reporting to test reporter
	r := traceview.SetTestReporter()

	ctx := NewContext(context.Background(), NewTrace("TestNullSpan")) // reports event
	l1, ctxL := BeginLayer(ctx, "L1")                                 // reports event
	assert.True(t, l1.IsTracing())
	assert.Equal(t, l1.MetadataString(), MetadataString(ctxL))
	assert.Len(t, l1.MetadataString(), 58)

	l1.End() // reports event
	assert.False(t, l1.IsTracing())
	assert.Empty(t, l1.MetadataString())

	p1 := l1.BeginProfile("P2") // try to start profile after end: no effect
	p1.End()

	c1 := l1.BeginLayer("C1") // child after parent ended
	assert.IsType(t, c1, &nullSpan{})
	assert.False(t, c1.IsTracing())
	assert.False(t, c1.ok())
	assert.Empty(t, c1.MetadataString())
	c1.addChildEdge(l1.tvContext())
	c1.addProfile(p1)

	nctx := c1.tvContext()
	assert.Equal(t, reflect.TypeOf(nctx).Elem().Name(), "nullContext")
	assert.IsType(t, reflect.TypeOf(nctx.Copy()).Elem().Name(), "nullContext")

	g.AssertGraph(t, r.Bufs, 3, g.AssertNodeMap{
		{"TestNullSpan", "entry"}: {},
		{"L1", "entry"}:           {Edges: g.Edges{{"TestNullSpan", "entry"}}},
		{"L1", "exit"}:            {Edges: g.Edges{{"L1", "entry"}}},
	})
}
