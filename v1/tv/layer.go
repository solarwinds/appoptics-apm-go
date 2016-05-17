// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import (
	"runtime"
	"runtime/debug"

	"golang.org/x/net/context"

	"github.com/appneta/go-traceview/v1/tv/internal/traceview"
)

type Layer interface {
	// End ends a layer, optionally reporting KV pairs provided by args.
	End(args ...interface{})
	BeginLayer(layerName string, args ...interface{}) Layer
	BeginProfile(profileName string, args ...interface{}) Profile

	Info(args ...interface{})
	Error(class, msg string)
	Err(error)

	addChildEdge(traceview.SampledContext)
	addProfile(Profile)
	tvContext() traceview.SampledContext
	ok() bool
}
type Profile interface {
	// End ends a profile, optionally reporting KV pairs provided by args.
	End(args ...interface{})
	Error(class, msg string)
	Err(error)
}

// BeginLayer starts a traced layer, returning an object to be used for reporting
// events attributed to or following from that layer.
func BeginLayer(ctx context.Context, layerName string, args ...interface{}) (Layer, context.Context) {
	if parent, ok := FromContext(ctx); ok { // report layer entry from parent context
		l := newLayer(parent.tvContext().Copy(), layerName, parent, args...)
		return l, newLayerContext(ctx, l)
	}
	return &nullSpan{}, ctx
}

func (l *layerSpan) BeginLayer(layerName string, args ...interface{}) Layer {
	if l.ok() { // copy parent context and report entry from child
		return newLayer(l.tvCtx.Copy(), layerName, l, args...)
	}
	return &nullSpan{}
}

// Begin a profiled block or method and return a context that should be closed with End().
// You can use defer to profile inside a method or block's scope, as below:
//   func exampleFunc(ctx context.Context) {
//       defer tv.BeginProfile(ctx, "exampleFunc").EndProfile()
//       // ... do something ...
//    }
func BeginProfile(ctx context.Context, profileName string, args ...interface{}) Profile {
	if parent, ok := FromContext(ctx); ok { // report profile entry from parent context
		return newProfile(parent.tvContext().Copy(), profileName, parent, args...)
	}
	return &nullSpan{}
}

// Begin a profiled block or method and return a context that should be closed with End().
func (l *layerSpan) BeginProfile(profileName string, args ...interface{}) Profile {
	if l.ok() { // copy parent context and report entry from child
		return newProfile(l.tvCtx.Copy(), profileName, l, args...)
	}
	return &nullSpan{}
}

// span satisfies the Extent interface and consolidates common reporting routines used by
// both Layer and Profile interfaces.
type span struct {
	labeler
	tvCtx         traceview.SampledContext
	parent        Layer
	childEdges    []traceview.SampledContext // for reporting in exit event
	childProfiles []Profile
	ended         bool // has exit event been reported?
}
type layerSpan struct{ span }   // satisfies Layer
type profileSpan struct{ span } // satisfies Profile
type nullSpan struct{}          // a span that is not tracing; satisfies Layer & Profile

func (_ *nullSpan) BeginLayer(layerName string, args ...interface{}) Layer { return &nullSpan{} }
func (_ *nullSpan) BeginProfile(name string, args ...interface{}) Profile  { return &nullSpan{} }
func (_ *nullSpan) End(args ...interface{})                                {}
func (_ *nullSpan) Error(class, msg string)                                {}
func (_ *nullSpan) Err(err error)                                          {}
func (_ *nullSpan) Info(args ...interface{})                               {}
func (_ *nullSpan) addChildEdge(traceview.SampledContext)                  {}
func (_ *nullSpan) addProfile(Profile)                                     {}
func (_ *nullSpan) ok() bool                                               { return false }
func (_ *nullSpan) tvContext() traceview.SampledContext                    { return &traceview.NullContext{} }

// is this layer still valid (has it timed out, expired, not sampled)
func (l *span) ok() bool                            { return l != nil && !l.ended }
func (t *span) tvContext() traceview.SampledContext { return t.tvCtx }

// addChildEdge keep track of edges closed child spans
func (s *span) addChildEdge(ctx traceview.SampledContext) {
	if s.ok() {
		s.childEdges = append(s.childEdges, ctx)
	}
}
func (s *layerSpan) addProfile(p Profile) {
	s.childProfiles = append([]Profile{p}, s.childProfiles...)
}

// labelers help spans choose label and layer names.
type labeler interface {
	entryLabel() traceview.Label
	exitLabel() traceview.Label
	layerName() string
}
type layerLabeler struct{ name string }
type profileLabeler struct{ name string }

// TV's "layer" and "profile" spans report their layer and label names slightly differently
func (_ layerLabeler) entryLabel() traceview.Label { return traceview.LabelEntry }
func (_ layerLabeler) exitLabel() traceview.Label  { return traceview.LabelExit }
func (l layerLabeler) layerName() string           { return l.name }
func newLayer(tvCtx traceview.SampledContext, layerName string, parent Layer, args ...interface{}) *layerSpan {
	ll := layerLabeler{layerName}
	tvCtx.ReportEvent(ll.entryLabel(), ll.layerName(), args...)
	return &layerSpan{span: span{tvCtx: tvCtx.Copy(), labeler: ll, parent: parent}}
}

func (_ profileLabeler) entryLabel() traceview.Label { return traceview.LabelProfileEntry }
func (_ profileLabeler) exitLabel() traceview.Label  { return traceview.LabelProfileExit }
func (_ profileLabeler) layerName() string           { return "" }
func newProfile(tvCtx traceview.SampledContext, profileName string, parent Layer, args ...interface{}) *profileSpan {
	var fname string
	pc, file, line, ok := runtime.Caller(2) // Caller(1) is BeginProfile
	if ok {
		f := runtime.FuncForPC(pc)
		fname = f.Name()
	}
	pl := profileLabeler{profileName}
	tvCtx.ReportEvent(pl.entryLabel(), pl.layerName(), // report profile entry
		"Language", "go", "ProfileName", profileName,
		"FunctionName", fname, "File", file, "LineNumber", line,
	)
	p := &profileSpan{span{tvCtx: tvCtx.Copy(), labeler: pl, parent: parent}}
	if parent != nil && parent.ok() {
		parent.addProfile(p)
	}
	return p
}

// End a profiled block or method.
func (s *span) End(args ...interface{}) {
	if s.ok() {
		for _, prof := range s.childProfiles {
			prof.End()
		}
		for _, edge := range s.childEdges { // add Edge KV for each joined child
			args = append(args, "Edge", edge)
		}
		s.tvCtx.ReportEvent(s.exitLabel(), s.layerName(), args...)
		s.childEdges = nil // clear child edge list
		s.ended = true
		// add this span's context to list to be used as Edge by parent exit
		if s.parent != nil && s.parent.ok() {
			s.parent.addChildEdge(s.tvCtx)
		}
	}
}

// Info reports KV pairs provided by args.
func (s *layerSpan) Info(args ...interface{}) {
	if s.ok() {
		s.tvCtx.ReportEvent(traceview.LabelInfo, s.layerName(), args...)
	}
}

// Error reports an error, distinguished by its class and message
func (s *span) Error(class, msg string) {
	if s.ok() {
		s.tvCtx.ReportEvent(traceview.LabelError, s.layerName(), "ErrorClass", class, "ErrorMsg", msg, "Backtrace", debug.Stack())
	}
}

// Err reports the provided error type
func (s *span) Err(err error) {
	if s.ok() && err != nil {
		s.tvCtx.ReportEvent(traceview.LabelError, s.layerName(), "ErrorClass", "error", "ErrorMsg", err.Error(), "Backtrace", debug.Stack())
	}
}
