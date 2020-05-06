package opentelemetry

import (
	"context"

	oteltrace "go.opentelemetry.io/otel/api/trace"
)

type tracer struct {
	// TODO
}

func (t *tracer) WithSpan(ctx context.Context, name string,
	body func(context.Context) error, opts ...oteltrace.StartOption) error {
	// TODO
	return nil
}

func (t *tracer) Start(ctx context.Context, name string, opts ...oteltrace.StartOption) (context.Context, oteltrace.Span) {
	// TODO
	return nil, nil
}
