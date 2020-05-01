package trace

import (
	"context"

	ot "go.opentelemetry.io/otel/sdk/export/trace"
)

// Exporter exports the OpenTelemetry trace data to the AppOptics backend. It's
// an implementation of the OpenTelemetry's trace.SpanSyncer interface.
type Exporter struct {
	// TODO
}

// Options are the options for initializing the AppOptics OpenTelemetry exporter.
type Options struct {
	// TODO
}

// NewExporter creates and returns a new Exporter instance
func NewExporter(o Options) (*Exporter, error) {
	// TODO
	return nil, nil
}

// ExportSpan converts the data of the OpenTelemetry trace and send it to the
// AppOptics backend.
func (e *Exporter) ExportSpan(ctx context.Context, data *ot.SpanData) {
	// TODO
}
