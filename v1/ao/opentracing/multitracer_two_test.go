// +build basictracer
//
// Behind a build tag avoid adding a dependency on basictracer-go

package opentracing

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	mt "github.com/appoptics/appoptics-apm-go/v1/contrib/multitracer"
	bt "github.com/opentracing/basictracer-go"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/harness"
)

// This test sets up AO and the OT "BasicTracer" side by side
func TestMultiTracerBasicTracerAPICheck(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true)) // set up test reporter
	harness.RunAPIChecks(t, func() (tracer opentracing.Tracer, closer func()) {
		return &mt.MultiTracer{
			Tracers: []opentracing.Tracer{
				NewTracer(),
				bt.NewWithOptions(bt.Options{
					Recorder:     bt.NewInMemoryRecorder(),
					ShouldSample: func(traceID uint64) bool { return true }, // always sample
				}),
			}}, nil
	},
		harness.CheckBaggageValues(false),
		harness.CheckInject(true),
	)
}
