// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import "golang.org/x/net/context"

var contextKey = "github.com/appneta/go-traceview/v1/tv.Trace"
var contextLayerKey = "github.com/appneta/go-traceview/v1/tv.Layer"

// NewContext returns a copy of the parent context and associates it with a Trace.
func NewContext(ctx context.Context, t Trace) context.Context {
	return context.WithValue(context.WithValue(ctx, contextKey, t), contextLayerKey, t)
}

// newLayerContext returns a copy of the parent context and associates it with a Layer.
func newLayerContext(ctx context.Context, l Layer) context.Context {
	return context.WithValue(ctx, contextLayerKey, l)
}

// FromContext returns the Layer bound to the context, if any.
func FromContext(ctx context.Context) (l Layer, ok bool) {
	l, ok = ctx.Value(contextLayerKey).(Layer)
	return
}

// TraceFromContext returns the Trace bound to the context, if any.
func TraceFromContext(ctx context.Context) (t Trace, ok bool) {
	t, ok = ctx.Value(contextKey).(Trace)
	return
}

// if context contains a valid Layer, run f
func runCtx(ctx context.Context, f func(l Layer)) {
	if l, ok := FromContext(ctx); ok {
		f(l)
	}
}

// if context contains a valid Trace, run f
func runTraceCtx(ctx context.Context, f func(t Trace)) {
	if t, ok := TraceFromContext(ctx); ok {
		f(t)
	}
}

// EndTrace reports the exit event for the layer name that was used when calling NewTrace(),
// given a context that was associated with trace.
// XXX return ctx with Trace removed?
func EndTrace(ctx context.Context) { runTraceCtx(ctx, func(t Trace) { t.End() }) }

// tv.End(ctx)
// XXX remove current Layer from ctx?
func End(ctx context.Context, args ...interface{}) { runCtx(ctx, func(l Layer) { l.End(args...) }) }

// tv.Info reports KV pairs provided by args on the root span.
func Info(ctx context.Context, args ...interface{}) { runCtx(ctx, func(l Layer) { l.Info(args...) }) }
func Error(ctx context.Context, class, msg string)  { runCtx(ctx, func(l Layer) { l.Error(class, msg) }) }
func Err(ctx context.Context, err error)            { runCtx(ctx, func(l Layer) { l.Err(err) }) }
