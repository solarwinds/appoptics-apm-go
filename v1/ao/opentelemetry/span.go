package opentelemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/api/core"
	oteltrace "go.opentelemetry.io/otel/api/trace"
	"google.golang.org/grpc/codes"
)

// TODO test
type span struct {
	// TODO
}

func (s *span) Tracer() oteltrace.Tracer {
	// TODO
	return nil
}

func (s *span) End(options ...oteltrace.EndOption) {
	// TODO
}

func (s *span) AddEvent(ctx context.Context, name string, attrs ...core.KeyValue) {
	// TODO
}

func (s *span) AddEventWithTimestamp(ctx context.Context, timestamp time.Time, name string, attrs ...core.KeyValue) {
	// TODO
}

func (s *span) IsRecording() bool {
	// TODO
	return false
}

func (s *span) RecordError(ctx context.Context, err error, opts ...oteltrace.ErrorOption) {
	// TODO
}

func (s *span) SpanContext() core.SpanContext {
	// TODO
	return core.SpanContext{}
}

func (s *span) SetStatus(codes.Code, string) {
	// TODO
}

func (s *span) SetName(name string) {
	// TODO
}

func (s *span) SetAttributes(...core.KeyValue) {
	// TODO
}

func (s *span) SetAttribute(string, interface{}) {
	// TODO
}
