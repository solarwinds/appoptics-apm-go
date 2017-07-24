// Copyright (C) 2017 Librato, Inc. All rights reserved.

package ottv

import (
	"sync"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/librato/go-traceview/v1/tv"
)

// NewTracer returns a new Tracelytics tracer.
func NewTracer() ot.Tracer {
	return &Tracer{
		textMapPropagator: &textMapPropagator{},
		binaryPropagator:  &binaryPropagator{marshaler: &jsonMarshaler{}},
	}
}

// Tracer reports trace data to Tracelytics.
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
	return t.StartSpanWithOptions(operationName, sso)
}

func (t *Tracer) StartSpanWithOptions(operationName string, opts ot.StartSpanOptions) ot.Span {
	// check if trace has already started (use Trace if there is no parent, Layer otherwise)
	// XXX handle StartTime

	for _, ref := range opts.References {
		switch ref.Type {
		// trace has parent XXX only handles one parent
		case ot.ChildOfRef, ot.FollowsFromRef:
			refCtx := ref.ReferencedContext.(spanContext)
			if refCtx.layer == nil { // referenced spanContext created by Extract()
				var layer tv.Layer
				if refCtx.sampled {
					layer = tv.NewTraceFromID(operationName, refCtx.remoteMD, func() tv.KVMap {
						return translateTags(opts.Tags)
					})
				} else {
					layer = tv.NewNullTrace()
				}
				return &spanImpl{tracer: t, context: spanContext{
					layer:   layer,
					sampled: refCtx.sampled,
					baggage: refCtx.baggage,
				},
				}
			}
			// referenced spanContext was in-process
			return &spanImpl{tracer: t, context: spanContext{layer: refCtx.layer.BeginLayer(operationName)}}
		}
	}

	// otherwise, no parent span found, so make new trace and return as span
	newSpan := &spanImpl{tracer: t, context: spanContext{layer: tv.NewTrace(operationName)}}
	return newSpan
}

type spanContext struct {
	// 1. spanContext created by StartSpanWithOptions
	layer tv.Layer
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

func (s *spanImpl) SetBaggageItem(key, val string) ot.Span {
	if !s.context.sampled && s.tracer.TrimUnsampledSpans {
		return s
	}

	s.Lock()
	defer s.Unlock()
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
	return spanContext{c.layer, c.remoteMD, c.sampled, newBaggage}
}

func (s *spanImpl) BaggageItem(key string) string {
	s.Lock()
	defer s.Unlock()
	return s.context.baggage[key]
}

const otLogPrefix = "OT-Log-"

func (s *spanImpl) LogFields(fields ...log.Field) {
	for _, field := range fields {
		s.context.layer.AddEndArgs(otLogPrefix+field.Key(), field.Value())
	}
}
func (s *spanImpl) LogKV(keyVals ...interface{}) { s.context.layer.AddEndArgs(keyVals...) }
func (s *spanImpl) Context() ot.SpanContext      { return s.context }
func (s *spanImpl) Finish()                      { s.context.layer.End() }
func (s *spanImpl) Tracer() ot.Tracer            { return s.tracer }

// XXX handle FinishTime, LogRecords
func (s *spanImpl) FinishWithOptions(opts ot.FinishOptions) { s.context.layer.End() }

// XXX handle changing operation name
func (s *spanImpl) SetOperationName(operationName string) ot.Span { return s }

func (s *spanImpl) SetTag(key string, value interface{}) ot.Span {
	s.context.layer.AddEndArgs(translateTagName(key), value)
	return s
}

// deprecated Log methods are not supported.
func (s *spanImpl) LogEvent(event string)                                 {}
func (s *spanImpl) LogEventWithPayload(event string, payload interface{}) {}
func (s *spanImpl) Log(data ot.LogData)                                   {}
