// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/harness"
)

// apiCheckProbe exposes methods for testing data recorded by a Tracer.
type apiCheckProbe struct{}

// SameTrace helps tests assert that this tracer's spans are from the same trace.
func (apiCheckProbe) SameTrace(first, second opentracing.Span) bool {
	sp1 := first.(*spanImpl)
	sp2 := second.(*spanImpl)
	return sp1.context.trace.LoggableTraceID() == sp2.context.trace.LoggableTraceID()
}

// SameSpanContext helps tests assert that a span and a context are from the same trace and span.
func (apiCheckProbe) SameSpanContext(span opentracing.Span, spanCtx opentracing.SpanContext) bool {
	sp := span.(*spanImpl)
	sc := spanCtx.(spanContext)
	var md1, md2 string
	md1 = sp.context.span.MetadataString()
	if sc.span == nil {
		md1 = sc.remoteMD
	} else {
		md1 = sc.span.MetadataString()
	}
	md2 = sp.context.span.MetadataString()
	return md1 == md2
}

func TestAPICheck(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true)) // set up test reporter
	harness.RunAPIChecks(t, func() (tracer opentracing.Tracer, closer func()) {
		return NewTracer(), nil
	},
		harness.CheckBaggageValues(true),
		harness.CheckInject(true),
		harness.CheckExtract(true),
		harness.UseProbe(apiCheckProbe{}),
	)
}
