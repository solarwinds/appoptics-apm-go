// Copyright (C) 2017 Librato, Inc. All rights reserved.

package ottv

import (
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/tracelytics/go-traceview/v1/tv/internal/traceview"
)

func TestSpanBaggageUnsampled(t *testing.T) {
	_ = traceview.SetTestReporter()
	tr := NewTracer()
	tr.(*Tracer).TrimUnsampledSpans = true
	span := tr.StartSpan("op")
	assert.NotNil(t, span)

	sp := span.SetBaggageItem("key", "val")
	assert.NotNil(t, sp)

	childSpan := tr.StartSpan("op2", opentracing.ChildOf(sp.Context()))
	assert.NotNil(t, childSpan)
}
