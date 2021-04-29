package opentelemetry

import (
	"go.opentelemetry.io/otel/trace"
)

// tpImpl implements the Provider interface of OpenTelemetry trace.
type tpImpl struct{}

// TPConfig defines the options for creating a Provider object.
type TPConfig struct {
}

// TPOption defines the function type to update the Tracer provider options.
type TPOption func(*TPConfig)

// make sure tpImpl implements the Provider interface.
var _ trace.TracerProvider = &tpImpl{}

// NewTracerProvider returns a new tpImpl object.
func NewTracerProvider(opts ...TPOption) (*tpImpl, error) {
	return &tpImpl{}, nil
}

// Tracer creates and returns a Tracer object.
func (p *tpImpl) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	if name == "" {
		name = "AppOptics"
	}
	return &tracerImpl{name: name}
}
