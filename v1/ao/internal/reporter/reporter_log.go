package reporter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/pkg/errors"
)

func newLogReporter() reporter {
	return &logReporter{
		dest:          os.Stderr,
		customMetrics: metrics.NewMeasurements(true, grpcMetricIntervalDefault, 500),
	}
}

type ServerlessMessage struct {
	Host    string   `json:"ao-host"`
	Service string   `json:"ao-service"`
	Data    []string `json:"ao-data"`
}

type logReporter struct {
	host          string // TODO
	service       string // TODO
	dest          *os.File
	customMetrics *metrics.Measurements
}

func (lr *logReporter) encode(bytes []byte) ([]byte, error) {
	msg := ServerlessMessage{
		Host:    lr.host,
		Service: lr.service,
		Data:    []string{"e:" + base64.StdEncoding.EncodeToString(bytes)},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding message")
	}
	return data, nil
}

func (lr *logReporter) write(bytes []byte) error {
	data, err := lr.encode(bytes)
	if err != nil {
		return errors.Wrap(err, "write to log reporter failed")
	}
	if _, err := lr.dest.Write(data); err != nil {
		return errors.Wrap(err, "write to log reporter failed")
	}
	return nil
}

func (lr *logReporter) reportEvent(ctx *oboeContext, e *event) error {
	if lr.Closed() {
		return ErrReporterIsClosed
	}
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	return lr.write((*e).bbuf.GetBuf())
}

// called when a status (e.g. __Init message) should be reported
func (lr *logReporter) reportStatus(ctx *oboeContext, e *event) error {
	return lr.reportEvent(ctx, e)
}

// called when a Span message should be reported
func (lr *logReporter) reportSpan(span metrics.SpanMessage) error {
	return nil // TODO
}

// Shutdown closes the reporter.
func (lr *logReporter) Shutdown(ctx context.Context) error {
	return nil
}

// ShutdownNow closes the reporter immediately
func (lr *logReporter) ShutdownNow() error {
	return nil
}

// Closed returns if the reporter is already closed.
func (lr *logReporter) Closed() bool {
	return false // never close
}

// WaitForReady waits until the reporter becomes ready or the context is canceled.
func (lr *logReporter) WaitForReady(context.Context) bool {
	return true
}

// CustomSummaryMetric submits a summary type measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func (lr *logReporter) CustomSummaryMetric(name string, value float64, opts metrics.MetricOptions) error {
	return lr.customMetrics.Summary(name, value, opts)
}

// CustomIncrementMetric submits a incremental measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func (lr *logReporter) CustomIncrementMetric(name string, opts metrics.MetricOptions) error {
	return lr.customMetrics.Increment(name, opts)
}
