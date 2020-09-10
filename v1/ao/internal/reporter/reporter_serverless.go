package reporter

import (
	"context"
	"io"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
)

type serverlessReporter struct {
	customMetrics *metrics.Measurements
	// the event writer for AWS Lambda
	logWriter FlushWriter
	// the http span
	span metrics.HTTPSpanMessage
}

func newServerlessReporter(writer io.Writer) reporter {
	r := &serverlessReporter{
		customMetrics: metrics.NewMeasurements(true, 0, 500),
	}

	r.logWriter = newLogWriter(false, writer, 260_000)

	updateSetting(int32(TYPE_DEFAULT),
		"",
		[]byte("OVERRIDE,SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		int64(config.GetSampleRate()), 120,
		argsToMap(float64(config.GetTokenBucketCap()),
			float64(config.GetTokenBucketRate()),
			20.000000,
			1.000000,
			6.000000,
			0.100000,
			60,
			-1,
			[]byte("")))

	log.Warningf("The reporter (v%v, go%v) for Lambda is ready.", utils.Version(), utils.GoVersion())

	return r
}

// called when an event should be reported
func (sr *serverlessReporter) reportEvent(ctx *oboeContext, e *event) error {
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	_, err := sr.logWriter.Write(EventWT, (*e).bbuf.GetBuf())
	return err
}

// called when a status (e.g. __Init message) should be reported
func (sr *serverlessReporter) reportStatus(ctx *oboeContext, e *event) error {
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	_, err := sr.logWriter.Write(EventWT, (*e).bbuf.GetBuf())
	return err
}

// called when a Span message should be reported
func (sr *serverlessReporter) reportSpan(span metrics.SpanMessage) error {
	if httpSpan, ok := span.(*metrics.HTTPSpanMessage); ok {
		sr.span = *httpSpan
	}
	return nil
}

// Shutdown closes the reporter.
func (sr *serverlessReporter) Shutdown(ctx context.Context) error {
	return sr.ShutdownNow()
}

// ShutdownNow closes the reporter immediately
func (sr *serverlessReporter) ShutdownNow() error {
	return nil
}

// Closed returns if the reporter is already closed.
func (sr *serverlessReporter) Closed() bool {
	return false
}

// WaitForReady waits until the reporter becomes ready or the context is canceled.
func (sr *serverlessReporter) WaitForReady(context.Context) bool {
	return true
}

// CustomSummaryMetric submits a summary type measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func (sr *serverlessReporter) CustomSummaryMetric(name string, value float64, opts metrics.MetricOptions) error {
	return sr.customMetrics.Summary(name, value, opts)
}

// CustomIncrementMetric submits a incremental measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func (sr *serverlessReporter) CustomIncrementMetric(name string, opts metrics.MetricOptions) error {
	return sr.customMetrics.Increment(name, opts)
}

func (sr *serverlessReporter) sendServerlessMetrics() {
	var messages [][]byte
	inbound := metrics.BuildServerlessMessage(sr.span)
	if inbound != nil {
		messages = append(messages, inbound)
	}

	custom := metrics.BuildMessage(sr.customMetrics.CopyAndReset(0), true)
	if custom != nil {
		messages = append(messages, custom)
	}

	sr.sendMetrics(messages)
}

func (sr *serverlessReporter) sendMetrics(msgs [][]byte) {
	for _, msg := range msgs {
		if _, err := sr.logWriter.Write(MetricWT, msg); err != nil {
			log.Warningf("sendMetrics: %s", err)
		}
	}
	return
}

func (sr *serverlessReporter) Flush() error {
	sr.sendServerlessMetrics()
	return sr.logWriter.Flush()
}
