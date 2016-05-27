// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import (
	"runtime"
	"runtime/debug"

	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"golang.org/x/net/context"
)

// Layer is used to measure a span of time associated with an actvity
// such as an RPC call, DB query, or method invocation.
type Layer interface {
	// BeginLayer starts a new layer span, returning a child of this Layer.
	BeginLayer(layerName string, args ...interface{}) Layer
	// BeginProfile starts a new profile, used to measure a named span of time spent in this Layer.
	BeginProfile(profileName string, args ...interface{}) Profile
	// End ends a layer, optionally reporting KV pairs provided by args.
	End(args ...interface{})
	// Add additional KV pairs that will be serialized (and dereferenced, for pointer
	// values) at the end of this trace's span.
	AddEndArgs(args ...interface{})

	// Info reports KV pairs provided by args for this Layer.
	Info(args ...interface{})
	// Error reports details about an error (along with a stack trace) for this Layer.
	Error(class, msg string)
	// Err reports details about error err (along with a stack trace) for this Layer.
	Err(error)

	// MetadataString returns a string representing this Layer for use in distributed tracing,
	// e.g. to provide as an "X-Trace" header in an outgoing HTTP request.
	MetadataString() string

	IsTracing() bool
	addChildEdge(traceview.Context)
	addProfile(Profile)
	tvContext() traceview.Context
	ok() bool
}

// Profile is used to provide micro-benchmarks of named timings inside a layer.
type Profile interface {
	// End ends a profile, optionally reporting KV pairs provided by args.
	End(args ...interface{})
	// Error reports details about an error (along with a stack trace) for this Profile.
	Error(class, msg string)
	// Err reports details about error err (along with a stack trace) for this Profile.
	Err(error)
}

// BeginLayer starts a new layer span, provided a parent context and name. It returns a Layer
// and context bound to the new child Layer.
func BeginLayer(ctx context.Context, layerName string, args ...interface{}) (Layer, context.Context) {
	if parent, ok := fromContext(ctx); ok && parent.ok() { // report layer entry from parent context
		l := newLayer(parent.tvContext().Copy(), layerName, parent, args...)
		return l, newLayerContext(ctx, l)
	}
	return &nullSpan{}, ctx
}

// BeginLayer starts a new layer span, returning a child of this Layer.
func (s *layerSpan) BeginLayer(layerName string, args ...interface{}) Layer {
	if s.ok() { // copy parent context and report entry from child
		return newLayer(s.tvCtx.Copy(), layerName, s, args...)
	}
	return &nullSpan{}
}

// BeginProfile begins a profiled block or method and return a context that should be closed with End().
// You can use defer to profile a function in one line, as below:
//   func exampleFunc(ctx context.Context) {
//       defer tv.BeginProfile(ctx, "exampleFunc").End()
//       // ... do something ...
//    }
func BeginProfile(ctx context.Context, profileName string, args ...interface{}) Profile {
	if parent, ok := fromContext(ctx); ok && parent.ok() { // report profile entry from parent context
		return newProfile(parent.tvContext().Copy(), profileName, parent, args...)
	}
	return &nullSpan{}
}

// BeginProfile starts a new profile, used to measure a named span of time spent in this Layer.
// The returned Profile should be closed with End().
func (s *layerSpan) BeginProfile(profileName string, args ...interface{}) Profile {
	if s.ok() { // copy parent context and report entry from child
		return newProfile(s.tvCtx.Copy(), profileName, s, args...)
	}
	return &nullSpan{}
}

// span satisfies the Extent interface and consolidates common reporting routines used by
// both Layer and Profile interfaces.
type span struct {
	labeler
	tvCtx         traceview.Context
	parent        Layer
	childEdges    []traceview.Context // for reporting in exit event
	childProfiles []Profile
	endArgs       []interface{}
	ended         bool // has exit event been reported?
}
type layerSpan struct{ span }   // satisfies Layer
type profileSpan struct{ span } // satisfies Profile
type nullSpan struct{}          // a span that is not tracing; satisfies Layer & Profile

func (s *nullSpan) BeginLayer(layerName string, args ...interface{}) Layer { return &nullSpan{} }
func (s *nullSpan) BeginProfile(name string, args ...interface{}) Profile  { return &nullSpan{} }
func (s *nullSpan) End(args ...interface{})                                {}
func (t *nullSpan) AddEndArgs(args ...interface{})                         {}
func (s *nullSpan) Error(class, msg string)                                {}
func (s *nullSpan) Err(err error)                                          {}
func (s *nullSpan) Info(args ...interface{})                               {}
func (s *nullSpan) IsTracing() bool                                        { return false }
func (s *nullSpan) addChildEdge(traceview.Context)                         {}
func (s *nullSpan) addProfile(Profile)                                     {}
func (s *nullSpan) ok() bool                                               { return false }
func (s *nullSpan) tvContext() traceview.Context                           { return traceview.NewNullContext() }
func (s *nullSpan) MetadataString() string                                 { return "" }

// is this layer still valid (has it timed out, expired, not sampled)
func (s *span) ok() bool                     { return s != nil && !s.ended }
func (s *span) IsTracing() bool              { return s.ok() }
func (s *span) tvContext() traceview.Context { return s.tvCtx }

// addChildEdge keep track of edges closed child spans
func (s *span) addChildEdge(ctx traceview.Context) {
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
func (l layerLabeler) entryLabel() traceview.Label { return traceview.LabelEntry }
func (l layerLabeler) exitLabel() traceview.Label  { return traceview.LabelExit }
func (l layerLabeler) layerName() string           { return l.name }
func newLayer(tvCtx traceview.Context, layerName string, parent Layer, args ...interface{}) Layer {
	ll := layerLabeler{layerName}
	if err := tvCtx.ReportEvent(ll.entryLabel(), ll.layerName(), args...); err != nil {
		return &nullSpan{}
	}
	return &layerSpan{span: span{tvCtx: tvCtx.Copy(), labeler: ll, parent: parent}}

}
func (l profileLabeler) entryLabel() traceview.Label { return traceview.LabelProfileEntry }
func (l profileLabeler) exitLabel() traceview.Label  { return traceview.LabelProfileExit }
func (l profileLabeler) layerName() string           { return "" }
func newProfile(tvCtx traceview.Context, profileName string, parent Layer, args ...interface{}) Profile {
	var fname string
	pc, file, line, ok := runtime.Caller(2) // Caller(1) is BeginProfile
	if ok {
		f := runtime.FuncForPC(pc)
		fname = f.Name()
	}
	pl := profileLabeler{profileName}
	if err := tvCtx.ReportEvent(pl.entryLabel(), pl.layerName(), // report profile entry
		"Language", "go", "ProfileName", profileName,
		"FunctionName", fname, "File", file, "LineNumber", line,
	); err != nil {
		return &nullSpan{}
	}
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
		_ = s.tvCtx.ReportEvent(s.exitLabel(), s.layerName(), args...)
		s.childEdges = nil // clear child edge list
		s.ended = true
		// add this span's context to list to be used as Edge by parent exit
		if s.parent != nil && s.parent.ok() {
			s.parent.addChildEdge(s.tvCtx)
		}
	}
}

// Add KV pairs as variadic args that will be serialized (and dereferenced, for pointer
// values) at the end of this trace's span.
func (l *layerSpan) AddEndArgs(args ...interface{}) {
	if l.ok() {
		// ensure even number of args added
		if len(args)%2 == 1 {
			args = args[0 : len(args)-1]
		}
		l.endArgs = append(l.endArgs, args...)
	}
}

// Info reports KV pairs provided by args.
func (s *layerSpan) Info(args ...interface{}) {
	if s.ok() {
		_ = s.tvCtx.ReportEvent(traceview.LabelInfo, s.layerName(), args...)
	}
}

// MetadataString returns a representation of the Layer span's context for use with distributed
// tracing (to create a remote child span). If the Layer has ended, an empty string is returned.
func (s *layerSpan) MetadataString() string {
	if s.ok() {
		return s.tvCtx.MetadataString()
	}
	return ""
}

// Error reports an error, distinguished by its class and message
func (s *span) Error(class, msg string) {
	if s.ok() {
		_ = s.tvCtx.ReportEvent(traceview.LabelError, s.layerName(),
			"ErrorClass", class, "ErrorMsg", msg, "Backtrace", debug.Stack())
	}
}

// Err reports the provided error type
func (s *span) Err(err error) {
	if s.ok() && err != nil {
		_ = s.tvCtx.ReportEvent(traceview.LabelError, s.layerName(),
			"ErrorClass", "error", "ErrorMsg", err.Error(), "Backtrace", debug.Stack())
	}
}
