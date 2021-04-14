package opentelemetry

import (
	"sync"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type spanImpl struct {
	mu            sync.Mutex
	tracer        trace.Tracer
	aoSpan        ao.Span
	context       trace.SpanContext
	name          string
	spanKind trace.SpanKind
	statusCode    codes.Code
	statusMessage string
	attributes    []attribute.KeyValue
	links         []trace.Link
	parent        trace.Span
}

var _ trace.Span = (*spanImpl)(nil)

func Wrapper(aoSpan ao.Span) trace.Span {
	return &spanImpl{
		tracer:  nil,
		aoSpan:  aoSpan,
		context: MdStr2OTSpanContext(aoSpan.MetadataString()),
		name:    "",
	}
}

func (s *spanImpl) Tracer() trace.Tracer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracer
}

func (s *spanImpl) End(options ...trace.SpanOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var args []interface{}
	for _, link := range s.links {
		mdStr := OTSpanContext2MdStr(link.SpanContext)
		args = append(args, "Link", mdStr)
		for _, attr := range link.Attributes {
			args = append(args, mdStr + "_" + string(attr.Key), attr.Value.AsInterface())
		}
	}
	for _, attr := range s.attributes {
		args = append(args, string(attr.Key), attr.Value.AsInterface())
	}

	cfg := trace.NewSpanConfig(options...)
	if !cfg.Timestamp.IsZero() {
		args = append(args, "Timestamp_u", cfg.Timestamp.UnixNano()/1000)
	}
	s.aoSpan.End(args...)
}

func (s *spanImpl) 	AddEvent(name string, options ...trace.EventOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := trace.NewEventConfig(options...)
	s.addEventWithTimestamp(c.Timestamp, name, c.Attributes...)
}

func (s *spanImpl) addEventWithTimestamp(timestamp time.Time, name string, attrs ...attribute.KeyValue) {
	var args []interface{}
	args = append(args, "Name", name)
	for _, attr := range attrs {
		args = append(args, string(attr.Key), attr.Value.AsInterface())
	}
	if !timestamp.IsZero() {
		args = append(args, "Timestamp_u", timestamp.UnixNano()/1000)
	}
	s.aoSpan.Info(args...)
}

func (s *spanImpl) IsRecording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aoSpan.IsReporting()
}

func (s *spanImpl) RecordError(err error, opts ...trace.EventOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err == nil {
		return
	}

	cfg := trace.NewEventConfig(opts...)

	if cfg.Timestamp.IsZero() {
		cfg.Timestamp = time.Now()
	}

	s.aoSpan.ErrorWithOpts(ao.WithErrMsg(err.Error()), ao.WithErrBackTrace(true))
}

func (s *spanImpl) SpanContext() trace.SpanContext {
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

func (s *spanImpl) SetAttributes(attrs ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes = append(s.attributes, attrs...)
}

func (s *spanImpl) SetAttribute(k string, v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes = append(s.attributes, attribute.Any(k, v))
}

func (s *spanImpl) addLink(link trace.Link) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append(s.links, link)
}
