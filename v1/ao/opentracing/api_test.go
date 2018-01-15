// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/appoptics/appoptics-apm-go/v1/ao/opentracing/internal/harness"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/suite"
)

func TestAPICheck(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true)) // set up test reporter
	apiSuite := harness.NewAPICheckSuite(func() (tracer opentracing.Tracer, closer func()) {
		return NewTracer(), nil
	}, harness.APICheckCapabilities{
		CheckBaggageValues: true,
		CheckInject:        true,
		CheckExtract:       true,
	})
	suite.Run(t, apiSuite)
}
