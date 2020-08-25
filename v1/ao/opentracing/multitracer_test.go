package opentracing

import (
	"testing"

	//	aoot "github.com/appoptics/appoptics-apm-go/v1/ao/opentracing"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	mt "github.com/appoptics/appoptics-apm-go/v1/contrib/multitracer"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/harness"
)

func TestMultiTracerAPICheck(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true)) // set up test reporter
	harness.RunAPIChecks(t, func() (tracer opentracing.Tracer, closer func()) {
		return &mt.MultiTracer{Tracers: []opentracing.Tracer{NewTracer()}}, nil
	},
		harness.CheckBaggageValues(false),
		harness.CheckInject(true),
		harness.CheckExtract(true),
	)
}
