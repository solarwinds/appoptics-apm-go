package opentelemetry

import (
	"bytes"
	"context"
	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/trace"
)

// tracer implements the Tracer interface of OpenTelemetry
type tracer struct {
	name string
}

var _ trace.Tracer = &tracer{}

// WithSpan wraps the execution of the fn function with a spanImpl.
// It starts a new spanImpl, sets it as an active spanImpl in the context,
// executes the fn function and closes the spanImpl before returning the
// result of fn.
func (t *tracer) WithSpan(ctx context.Context, name string,
	body func(context.Context) error, opts ...trace.StartOption) error {
	ctx, span := t.Start(ctx, name, opts...)
	defer span.End()

	if err := body(ctx); err != nil {
		return err
	}
	return nil
}

// Start a span
func (t *tracer) Start(ctx context.Context, name string,
	o ...trace.StartOption) (context.Context, trace.Span) {
	var opts trace.StartConfig

	for _, op := range o {
		op(&opts)
	}

	parentSpan, remoteSpanCtx, links := GetSpanContextAndLinks(ctx, opts.NewRoot)
	s := &spanImpl{tracer: t}
	if parentSpan != nil {
		s.parent = parentSpan // TODO we don't seem to need it?
		parentAoSpan := parentSpan.(*spanImpl)
		s.aoSpan = parentAoSpan.aoSpan.BeginSpanWithOptions(name, ao.SpanOptions{
			StartTime: opts.StartTime,
		})
	} else {
		mdStr := ""
		if remoteSpanCtx != trace.EmptySpanContext() {
			mdStr = OTSpanContext2MdStr(remoteSpanCtx)
		}
		s.aoSpan = ao.NewTraceWithOptions(s.name, ao.SpanOptions{
			StartTime: opts.StartTime,
			ContextOptions: ao.ContextOptions{
				MdStr: mdStr,
			},
		})
	}

	ctx = trace.ContextWithSpan(ctx, s)

	for _, l := range links {
		s.addLink(l)
	}
	for _, l := range opts.Links {
		s.addLink(l)
	}

	return ctx, s
}

// GetSpanContextAndLinks get local and remote SpanContext from the context, and
// produces the links accordingly.
// This is based from the OpenTelemetry sdk code.
func GetSpanContextAndLinks(ctx context.Context,
	ignoreContext bool) (trace.Span, trace.SpanContext, []trace.Link) {
	local := trace.SpanFromContext(ctx)
	remoteContext := trace.RemoteSpanContextFromContext(ctx)

	if ignoreContext {
		links := addLinkIfValid(nil, local.SpanContext(), "current")
		links = addLinkIfValid(links, remoteContext, "remote")

		return nil, trace.EmptySpanContext(), links
	}
	if local.SpanContext().IsValid() {
		return local, trace.EmptySpanContext(), nil
	}
	if remoteContext.IsValid() {
		return nil, remoteContext, nil
	}
	return nil, trace.EmptySpanContext(), nil
}

func addLinkIfValid(links []trace.Link, sc trace.SpanContext, kind string) []trace.Link {
	if !sc.IsValid() {
		return links
	}
	return append(links, trace.Link{
		SpanContext: sc,
		Attributes: []core.KeyValue{
			key.String("ignored-on-demand", kind),
		},
	})
}

// OTSpanContext2MdStr generates the X-Trace ID from the OpenTelemetry span context.
func OTSpanContext2MdStr(context trace.SpanContext) string {
	if !context.IsValid() {
		return ""
	}
	return getXTraceID(context.SpanID, context.TraceID, context.IsSampled())
}

func MdStr2OTSpanContext(mdStr string) trace.SpanContext {
	// TODO
	//
	//  we are not able to convert it back seamlessly as the task ID will be
	// truncated and lose precision.
	return trace.EmptySpanContext()
}

func getXTraceID(spanID trace.SpanID, traceID trace.ID, sampled bool) string {
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
