package opentelemetry

import (
	"context"
	"sync"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/trace"
	"google.golang.org/grpc/codes"
)

// TODO test
type spanImpl struct {
	mu            sync.Mutex
	tracer        trace.Tracer
	aoSpan        ao.Span
	context       core.SpanContext
	name          string
	statusCode    codes.Code
	statusMessage string
	attributes    []core.KeyValue
	links         []trace.Link
	parent        trace.Span
}

var _ trace.Span = &spanImpl{}

func Wrapper(aoSpan ao.Span) trace.Span {
	return &spanImpl{
		tracer:  nil, // TODO no tracer for it, should be OK?
		aoSpan:  aoSpan,
		context: MdStr2OTSpanContext(aoSpan.MetadataString()),
		name:    "", // TODO expose AO span name
	}
}

func (s *spanImpl) Tracer() trace.Tracer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracer
}

func (s *spanImpl) End(options ...trace.EndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// TODO explicitly set span end time
	var args []interface{}
	for _, link := range s.links {
		args = append(args, "link", OTSpanContext2MdStr(link.SpanContext))
	}
	for _, attr := range s.attributes {
		args = append(args, string(attr.Key), attr.Value.AsInterface())
	}
	s.aoSpan.End(args...)
}

func (s *spanImpl) AddEvent(ctx context.Context, name string, attrs ...core.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addEventWithTimestamp(ctx, time.Now(), name, attrs...)
}

func (s *spanImpl) addEventWithTimestamp(ctx context.Context, timestamp time.Time,
	name string, attrs ...core.KeyValue) {
	// TODO explicitly set timestamp
	var args []interface{}
	args = append(args, "Name", name)
	for _, attr := range attrs {
		args = append(args, string(attr.Key), attr.Value.AsInterface())
	}
	s.aoSpan.Info(args...) // TODO add attrs
}

func (s *spanImpl) AddEventWithTimestamp(ctx context.Context, timestamp time.Time,
	name string, attrs ...core.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addEventWithTimestamp(ctx, timestamp, name, attrs...)
}

func (s *spanImpl) IsRecording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aoSpan.IsReporting()
}

func (s *spanImpl) RecordError(ctx context.Context, err error, opts ...trace.ErrorOption) {
	// Do not lock s.mu here otherwise there will be a deadlock as it calls s.SetStatus
	if err == nil {
		return
	}

	cfg := &trace.ErrorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.Timestamp.IsZero() {
		cfg.Timestamp = time.Now()
	}
	if cfg.StatusCode != codes.OK {
		s.SetStatus(cfg.StatusCode, "")
	}
	// TODO pass in ErrorOption (timestamp, etc)
	s.aoSpan.Err(err)
}

func (s *spanImpl) SpanContext() core.SpanContext {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.context
}

func (s *spanImpl) SetStatus(code codes.Code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCode = code
	s.statusMessage = msg
}

func (s *spanImpl) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

func (s *spanImpl) SetAttributes(attrs ...core.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes = append(s.attributes, attrs...)
}

func (s *spanImpl) SetAttribute(k string, v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes = append(s.attributes, key.Infer(k, v))
}

func (s *spanImpl) addLink(link trace.Link) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append(s.links, link)
}
