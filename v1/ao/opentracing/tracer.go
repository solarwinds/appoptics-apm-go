// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

// NewTracer returns a new AppOptics tracer.
func NewTracer() ot.Tracer {
	return &Tracer{
		textMapPropagator: &textMapPropagator{},
		binaryPropagator:  &binaryPropagator{marshaler: &jsonMarshaler{}},
	}
}

// Tracer reports trace data to AppOptics.
type Tracer struct {
	textMapPropagator  *textMapPropagator
	binaryPropagator   *binaryPropagator
	TrimUnsampledSpans bool
}

// StartSpan belongs to the Tracer interface.
func (t *Tracer) StartSpan(operationName string, opts ...ot.StartSpanOption) ot.Span {
	sso := ot.StartSpanOptions{}
	for _, o := range opts {
		o.Apply(&sso)
	}
	return t.startSpanWithOptions(operationName, sso)
}

func (t *Tracer) startSpanWithOptions(operationName string, opts ot.StartSpanOptions) ot.Span {
	// check if trace has already started (use Trace if there is no parent, Span otherwise)
	// XXX handle StartTime
	var newSpan ot.Span

	for _, ref := range opts.References {
		switch ref.Type {
		// trace has parent XXX only handles one parent
		case ot.ChildOfRef, ot.FollowsFromRef:
			refCtx := ref.ReferencedContext.(spanContext)
			if refCtx.span == nil { // referenced spanContext created by Extract()
				var span ao.Span
				if refCtx.sampled {
					span = ao.NewTraceFromID(operationName, refCtx.remoteMD, nil)
				} else {
					span = ao.NewNullTrace()
				}
				newSpan = &spanImpl{tracer: t, context: spanContext{
					span:    span,
					sampled: refCtx.sampled,
					baggage: refCtx.baggage,
				},
				}
			} else {
				// referenced spanContext was in-process
				newSpan = &spanImpl{tracer: t, context: spanContext{span: refCtx.span.BeginSpan(operationName)}}
			}
		}
	}

	// otherwise, no parent span found, so make new trace and return as span
	if newSpan == nil {
		newSpan = &spanImpl{tracer: t, context: spanContext{span: ao.NewTrace(operationName)}}
	}

	// add tags, if provided in span options
	for k, v := range opts.Tags {
		newSpan.SetTag(k, v)
	}

	return newSpan
}

type spanContext struct {
	// 1. spanContext created by StartSpanWithOptions
	span ao.Span
	// 2. spanContext created by Extract()
	remoteMD string
	sampled  bool

	// The span's associated baggage.
	baggage map[string]string // initialized on first use
}

type spanImpl struct {
	tracer     *Tracer
	sync.Mutex // protects the field below
	context    spanContext
}

// SetBaggageItem sets the KV as a baggage item.
func (s *spanImpl) SetBaggageItem(key, val string) ot.Span {
	s.Lock()
	defer s.Unlock()
	if !s.context.sampled && s.tracer.TrimUnsampledSpans {
		return s
	}

	s.context = s.context.WithBaggageItem(key, val)
	return s
}

// ForeachBaggageItem grants access to all baggage items stored in the SpanContext.
// The bool return value indicates if the handler wants to continue iterating
// through the rest of the baggage items.
func (c spanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	for k, v := range c.baggage {
		if !handler(k, v) {
			break
		}
	}
}

// WithBaggageItem returns an entirely new basictracer SpanContext with the
// given key:value baggage pair set.
func (c spanContext) WithBaggageItem(key, val string) spanContext {
	var newBaggage map[string]string
	if c.baggage == nil {
		newBaggage = map[string]string{key: val}
	} else {
		newBaggage = make(map[string]string, len(c.baggage)+1)
		for k, v := range c.baggage {
			newBaggage[k] = v
		}
		newBaggage[key] = val
	}
	// Use positional parameters so the compiler will help catch new fields.
	return spanContext{c.span, c.remoteMD, c.sampled, newBaggage}
}

// BaggageItem returns the baggage item with the provided key.
func (s *spanImpl) BaggageItem(key string) string {
	s.Lock()
	defer s.Unlock()
	return s.context.baggage[key]
}

const otLogPrefix = "OT-Log-"

// LogFields adds the fields to the span.
func (s *spanImpl) LogFields(fields ...log.Field) {
	s.Lock()
	defer s.Unlock()
	for _, field := range fields {
		s.context.span.AddEndArgs(otLogPrefix+field.Key(), field.Value())
	}
}

// LogKV adds KVs to the span.
func (s *spanImpl) LogKV(keyVals ...interface{}) {
	s.Lock()
	defer s.Unlock()
	s.context.span.AddEndArgs(keyVals...)
}

// Context returns the span context.
func (s *spanImpl) Context() ot.SpanContext {
	s.Lock()
	defer s.Unlock()
	return s.context
}

// Finish sets the end timestamp and finalizes Span state.
func (s *spanImpl) Finish() {
	s.Lock()
	defer s.Unlock()
	s.context.span.End()
}

// Tracer provides the tracer of this span.
func (s *spanImpl) Tracer() ot.Tracer { return s.tracer }

// FinishWithOptions is like Finish() but with explicit control over
// timestamps and log data.
// XXX handle FinishTime, LogRecords
func (s *spanImpl) FinishWithOptions(opts ot.FinishOptions) {
	s.Lock()
	defer s.Unlock()
	s.context.span.End()
}

// SetOperationName sets or changes the operation name.
func (s *spanImpl) SetOperationName(operationName string) ot.Span {
	s.Lock()
	defer s.Unlock()

	s.context.span.SetOperationName(operationName)
	return s
}

// SetTag adds a tag to the span.
func (s *spanImpl) SetTag(key string, value interface{}) ot.Span {
	s.Lock()
	defer s.Unlock()
	// if transaction name is passed, set on the span
	tagName := translateTagName(key)
	switch tagName {
	case "TransactionName":
		if txnName, ok := value.(string); ok {
			s.context.span.SetTransactionName(txnName)
		}
	case string(ext.Error):
		s.setErrorTag(value)
	default:
		s.context.span.AddEndArgs(tagName, value)
	}
	return s
}

// setErrorTag passes an OT error to the AO span.Error method.
func (s *spanImpl) setErrorTag(value interface{}) {
	switch v := value.(type) {
	case bool:
		// OpenTracing spec defines bool value
		if v {
			s.context.span.Error(string(ext.Error), "true")
		}
	case error:
		// handle error if provided
		s.context.span.Error(reflect.TypeOf(v).String(), v.Error())
	case string:
		// error string provided
		s.context.span.Error(string(ext.Error), v)
	case fmt.Stringer:
		s.context.span.Error(string(ext.Error), v.String())
	case nil:
		// no error, ignore
	default:
		// an unknown error type, assume an error
		s.context.span.Error(string(ext.Error), reflect.TypeOf(v).String())
	}
}

// LogEvent logs a event to the span.
//
// Deprecated: this method is deprecated.
func (s *spanImpl) LogEvent(event string) {}

// LogEventWithPayload logs a event with a payload.
//
// Deprecated: this method is deprecated.
func (s *spanImpl) LogEventWithPayload(event string, payload interface{}) {}

// Log logs the LogData.
//
// Deprecated: this method is deprecated.
func (s *spanImpl) Log(data ot.LogData) {}
