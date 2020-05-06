package opentelemetry

import (
	api "go.opentelemetry.io/otel/api/trace"
)

// TracerProvider implements the Provider interface of OpenTelemetry api.
type TracerProvider struct{}

// ProviderOptions defines the options for creating a Provider object.
type ProviderOptions struct {
}

type ProviderOption func(*ProviderOptions)

// make sure TracerProvider implements the Provider interface.
var _ api.Provider = &TracerProvider{}

// NewTracerProvider returns a new TracerProvider object.
func NewTracerProvider(opts ...ProviderOption) (*TracerProvider, error) {
	return &TracerProvider{}, nil
}

// Tracer creates and returns a Tracer object.
func (p *TracerProvider) Tracer(name string) api.Tracer {
	return &tracer{name: name}
}
