package opentelemetry

import (
	"go.opentelemetry.io/otel/api/trace"
)

// TracerProvider implements the Provider interface of OpenTelemetry trace.
type TracerProvider struct{}

// ProviderOptions defines the options for creating a Provider object.
type ProviderOptions struct {
}

// ProviderOption defines the function type to update the Tracer provider options.
type ProviderOption func(*ProviderOptions)

// make sure TracerProvider implements the Provider interface.
var _ trace.Provider = &TracerProvider{}

// NewTracerProvider returns a new TracerProvider object.
func NewTracerProvider(opts ...ProviderOption) (*TracerProvider, error) {
	return &TracerProvider{}, nil
}

// Tracer creates and returns a Tracer object.
func (p *TracerProvider) Tracer(name string) trace.Tracer {
	return &tracer{name: name}
}
