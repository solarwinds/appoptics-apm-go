package opentelemetry

import (
	"bytes"
	"context"
	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// tracerImpl implements the Tracer interface of OpenTelemetry
type tracerImpl struct {
	name string
}

var _ trace.Tracer = &tracerImpl{}

// Start a span
func (t *tracerImpl) Start(ctx context.Context, name string, options ...trace.SpanOption) (context.Context, trace.Span) {
	config := trace.NewSpanConfig(options...)

	parentSpan, links := GetSpanContextAndLinks(ctx, config.NewRoot)
	s := &spanImpl{tracer: t, name: name, spanKind: config.SpanKind}
	if parentSpan != nil {
		s.parent = parentSpan // TODO we don't seem to need it?
		parentAoSpan := parentSpan.(*spanImpl)
		s.aoSpan = parentAoSpan.aoSpan.BeginSpanWithOptions(s.name, ao.SpanOptions{
			StartTime: config.Timestamp,
		})
	} else {
		mdStr := ""
		s.aoSpan = ao.NewTraceWithOptions(s.name, ao.SpanOptions{
			StartTime: config.Timestamp,
			ContextOptions: ao.ContextOptions{
				MdStr: mdStr,
			},
		})
	}

	ctx = trace.ContextWithSpan(ctx, s)

	for _, l := range links {
		s.addLink(l)
	}
	for _, l := range config.Links {
		s.addLink(l)
	}
	s.SetAttributes(config.Attributes...)

	return ctx, s
}

// GetSpanContextAndLinks get local and remote SpanContext from the context, and
// produces the links accordingly.
// This is based from the OpenTelemetry sdk code.
func GetSpanContextAndLinks(ctx context.Context,
	ignoreContext bool) (trace.Span, []trace.Link) {
	local := trace.SpanFromContext(ctx)

	if ignoreContext {
		links := addLinkIfValid(nil, local.SpanContext(), "current")
		return nil, links
	}
	if _, ok := local.(*spanImpl); ok && local.SpanContext().IsValid() {
		return local, nil
	}

	return nil, nil
}

func addLinkIfValid(links []trace.Link, sc trace.SpanContext, kind string) []trace.Link {
	if !sc.IsValid() {
		return links
	}
	return append(links, trace.Link{
		SpanContext: sc,
		Attributes: []attribute.KeyValue{
			attribute.String("ignored-on-demand", kind),
		},
	})
}

// OTSpanContext2MdStr generates the X-Trace ID from the OpenTelemetry span context.
func OTSpanContext2MdStr(context trace.SpanContext) string {
	return getXTraceID(context.SpanID(), context.TraceID(), context.IsSampled())
}

func MdStr2OTSpanContext(mdStr string) trace.SpanContext {
	// TODO
	//
	// we are not able to convert it back seamlessly as the task ID will be
	// truncated and lose precision.
	return trace.SpanContext{}
}

func getXTraceID(spanID trace.SpanID, traceID trace.TraceID, sampled bool) string {
	buf := bytes.Buffer{}
	buf.WriteString("2B")

	traceIDStr := traceID.String()
	buf.WriteString(traceIDStr)
	buf.WriteString(strings.Repeat("0", reporter.OboeMaxTaskIDLen*2-len(traceIDStr)))
	spanIDStr := spanID.String()
	buf.WriteString(spanIDStr)
	buf.WriteString(strings.Repeat("0", reporter.OboeMaxOpIDLen*2-len(spanIDStr)))

	if sampled {
		buf.WriteString("01")
	} else {
		buf.WriteString("00")
	}
	return strings.ToUpper(buf.String())
}
