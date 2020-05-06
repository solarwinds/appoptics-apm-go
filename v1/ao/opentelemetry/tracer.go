package opentelemetry

import (
	"context"

	api "go.opentelemetry.io/otel/api/trace"
)

type tracer struct {
	name string
	// TODO
}

func (t *tracer) WithSpan(ctx context.Context, name string,
	body func(context.Context) error, opts ...api.StartOption) error {
	// TODO
	return nil
}

func (t *tracer) Start(ctx context.Context, name string,
	opts ...api.StartOption) (context.Context, api.Span) {
	// TODO
	return nil, nil
}
