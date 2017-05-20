// Copyright (C) 2017 Librato, Inc. All rights reserved.

package ottv

import (
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/suite"
	"github.com/tracelytics/go-traceview/v1/tv/internal/traceview"
	"github.com/tracelytics/go-traceview/v1/tv/ottv/internal/harness"
)

func TestAPICheck(t *testing.T) {
	_ = traceview.SetTestReporter() // set up test reporter
	apiSuite := harness.NewAPICheckSuite(func() (tracer opentracing.Tracer, closer func()) {
		return NewTracer(), nil
	}, harness.APICheckCapabilities{
		CheckBaggageValues: true,
		CheckInject:        true,
		CheckExtract:       true,
	})
	suite.Run(t, apiSuite)
}
