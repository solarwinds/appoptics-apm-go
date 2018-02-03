// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/harness"
)

func TestAPICheck(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true)) // set up test reporter
	harness.RunAPIChecks(t, func() (tracer opentracing.Tracer, closer func()) {
		return NewTracer(), nil
	},
		harness.CheckBaggageValues(true),
		harness.CheckInject(true),
		harness.CheckExtract(true),
	)
}
