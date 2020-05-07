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

	localParent, remoteContext, links := GetSpanContextAndLinks(ctx, opts.NewRoot)
	s := &spanImpl{tracer: t}
	if localParent != nil {
		s.parent = localParent
		parentAoSpan := localParent.(*spanImpl)
		s.aoSpan = parentAoSpan.aoSpan.BeginSpan(name)
	} else if remoteContext != core.EmptySpanContext() {
		s.aoSpan, ctx = ao.BeginSpanWithOptions(ctx, name, ao.SpanOptions{
			ContextOptions: ao.ContextOptions{
				MdStr: OTSpanContext2MdStr(remoteContext),
			},
		})
		ctx = trace.ContextWithSpan(ctx, s)
	} else {
		s.aoSpan = ao.NewTrace(name)
	}

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
func GetSpanContextAndLinks(ctx context.Context,
	ignoreContext bool) (trace.Span, core.SpanContext, []trace.Link) {
	local := trace.SpanFromContext(ctx)
	remoteContext := trace.RemoteSpanContextFromContext(ctx)

	if ignoreContext {
		links := addLinkIfValid(nil, local.SpanContext(), "current")
		links = addLinkIfValid(links, remoteContext, "remote")

		return nil, core.EmptySpanContext(), links
	}
	if local.SpanContext().IsValid() {
		return local, core.EmptySpanContext(), nil
	}
	if remoteContext.IsValid() {
		return nil, remoteContext, nil
	}
	return nil, core.EmptySpanContext(), nil
}

func addLinkIfValid(links []trace.Link, sc core.SpanContext, kind string) []trace.Link {
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
func OTSpanContext2MdStr(context core.SpanContext) string {
	return getXTraceID(context.SpanID, context.TraceID, context.IsSampled())
}

func getXTraceID(spanID core.SpanID, traceID core.TraceID, sampled bool) string {
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
