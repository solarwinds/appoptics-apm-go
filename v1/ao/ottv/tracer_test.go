// Copyright (C) 2017 Librato, Inc. All rights reserved.

package ottv

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
)

func TestSpanBaggageUnsampled(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true))
	tr := NewTracer()
	tr.(*Tracer).TrimUnsampledSpans = true
	span := tr.StartSpan("op")
	assert.NotNil(t, span)

	sp := span.SetBaggageItem("key", "val")
	assert.NotNil(t, sp)

	childSpan := tr.StartSpan("op2", opentracing.ChildOf(sp.Context()))
	assert.NotNil(t, childSpan)
}
