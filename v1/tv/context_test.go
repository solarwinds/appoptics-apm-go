// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/appneta/go-traceview/v1/tv/internal/traceview"
	"golang.org/x/net/context"
)

func TestContext(t *testing.T) {
	ctx := context.Background()
	tr := NewTrace("test").(*tvTrace)
	xt := tr.tvCtx.String()

	ctx2 := context.WithValue(ctx, "t", tr)
	assert.Equal(t, ctx2.Value("t"), tr)
	assert.Equal(t, ctx2.Value("t").(*tvTrace).tvCtx.String(), xt)

	ctxx := tr.tvCtx.Copy()
	lbl := layerLabeler{"L1"}
	tr2 := &tvTrace{layerSpan{span: span{tvCtx: ctxx, labeler: lbl}}, nil}
	ctx3 := context.WithValue(ctx2, "t", tr2)
	assert.Equal(t, ctx3.Value("t"), tr2)

	ctxx2 := tr2.tvCtx.Copy()
	tr3 := &tvTrace{layerSpan{span: span{tvCtx: ctxx2}}, nil}
	ctx4 := context.WithValue(ctx3, "t", tr3)
	assert.Equal(t, ctx4.Value("t"), tr3)
}

func TestNullSpan(t *testing.T) {
	ctx := NewContext(context.Background(), NewTrace("TestNullSpan"))
	l1, _ := BeginLayer(ctx, "L1")
	l1.End()

	c1 := l1.BeginLayer("C1") // child after parent ended
	assert.IsType(t, c1, &nullSpan{})
	assert.False(t, c1.ok())

	nctx := c1.tvContext()
	assert.IsType(t, nctx, &traceview.NullContext{})
	assert.IsType(t, nctx.Copy(), &traceview.NullContext{})
}
