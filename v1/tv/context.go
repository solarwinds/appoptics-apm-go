// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import "golang.org/x/net/context"

var contextKey = "github.com/appneta/go-appneta/v1/tv.Trace"
var contextLayerKey = "github.com/appneta/go-appneta/v1/tv.Layer"

// NewContext returns a copy of the parent context and associates it with a Trace.
func NewContext(ctx context.Context, t Trace) context.Context {
	return context.WithValue(context.WithValue(ctx, contextKey, t), contextLayerKey, t)
}

// newLayerContext returns a copy of the parent context and associates it with a Layer.
func newLayerContext(ctx context.Context, l Layer) context.Context {
	return context.WithValue(ctx, contextLayerKey, l)
}

// FromContext returns the Layer bound to the context, if any.
func FromContext(ctx context.Context) Layer {
	l, ok := fromContext(ctx)
	if !ok {
		return nullSpan{}
	}
	return l
}
func fromContext(ctx context.Context) (l Layer, ok bool) {
	l, ok = ctx.Value(contextLayerKey).(Layer)
	return
}

// TraceFromContext returns the Trace bound to the context, if any.
func TraceFromContext(ctx context.Context) Trace {
	t, ok := traceFromContext(ctx)
	if !ok {
		return nullTrace{}
	}
	return t
}
func traceFromContext(ctx context.Context) (t Trace, ok bool) {
	t, ok = ctx.Value(contextKey).(Trace)
	return
}

// if context contains a valid Layer, run f
func runCtx(ctx context.Context, f func(l Layer)) {
	if l, ok := fromContext(ctx); ok {
		f(l)
	}
}

// if context contains a valid Trace, run f
func runTraceCtx(ctx context.Context, f func(t Trace)) {
	if t, ok := traceFromContext(ctx); ok {
		f(t)
	}
}

// EndTrace ends a Trace, given a context that was associated with the trace.
func EndTrace(ctx context.Context) { runTraceCtx(ctx, func(t Trace) { t.End() }) }

// End ends a Layer, given a context ctx that was associated with it, optionally reporting KV pairs
// provided by args.
func End(ctx context.Context, args ...interface{}) { runCtx(ctx, func(l Layer) { l.End(args...) }) }

// Info reports KV pairs provided by args for the Layer associated with the context ctx.
func Info(ctx context.Context, args ...interface{}) { runCtx(ctx, func(l Layer) { l.Info(args...) }) }

// Error reports details about an error (along with a stack trace) on the Layer associated with the context ctx.
func Error(ctx context.Context, class, msg string) { runCtx(ctx, func(l Layer) { l.Error(class, msg) }) }

// Err reports details error err (along with a stack trace) on the Layer associated with the context ctx.
func Err(ctx context.Context, err error) { runCtx(ctx, func(l Layer) { l.Err(err) }) }

// MetadataString returns a representation of the Layer span's context for use with distributed
// tracing (to create a remote child span). If the Layer has ended, an empty string is returned.
func MetadataString(ctx context.Context) string {
	if l, ok := fromContext(ctx); ok {
		return l.MetadataString()
	}
	return ""
}
