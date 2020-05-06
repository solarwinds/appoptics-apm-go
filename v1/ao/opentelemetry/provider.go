package opentelemetry

import (
	oteltrace "go.opentelemetry.io/otel/api/trace"
)

type TracerProvider struct{}

func (p *TracerProvider) Tracer(name string) oteltrace.Tracer {
	// TODO
	return nil
}
