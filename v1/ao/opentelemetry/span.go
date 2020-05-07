package opentelemetry

import (
	"context"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"go.opentelemetry.io/otel/api/core"
	trace "go.opentelemetry.io/otel/api/trace"
	"google.golang.org/grpc/codes"
)

// TODO test
type spanImpl struct {
	tracer  trace.Tracer
	aoSpan  ao.Span
	context core.SpanContext
	links   []trace.Link
	parent  trace.Span
	// TODO
}

var _ trace.Span = &spanImpl{}

func (s *spanImpl) Tracer() trace.Tracer {
	return s.tracer
}

func (s *spanImpl) End(options ...trace.EndOption) {
	// TODO
}

func (s *spanImpl) AddEvent(ctx context.Context, name string, attrs ...core.KeyValue) {
	// TODO
}

func (s *spanImpl) AddEventWithTimestamp(ctx context.Context, timestamp time.Time,
	name string, attrs ...core.KeyValue) {
	// TODO
}

func (s *spanImpl) IsRecording() bool {
	// TODO
	return false
}

func (s *spanImpl) RecordError(ctx context.Context, err error, opts ...trace.ErrorOption) {
	// TODO
}

func (s *spanImpl) SpanContext() core.SpanContext {
	// TODO
	return core.SpanContext{}
}

func (s *spanImpl) SetStatus(codes.Code, string) {
	// TODO
}

func (s *spanImpl) SetName(name string) {
	// TODO
}

func (s *spanImpl) SetAttributes(...core.KeyValue) {
	// TODO
}

func (s *spanImpl) SetAttribute(string, interface{}) {
	// TODO
}

func (s *spanImpl) addLink(link trace.Link) {
	s.links = append(s.links, link)
}
