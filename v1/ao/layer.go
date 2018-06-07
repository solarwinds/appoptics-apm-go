// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"runtime"
	"runtime/debug"
	"sync"

	"errors"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"golang.org/x/net/context"
)

const (
	MaxCustomTransactionNameLength = 255
)

// Span is used to measure a span of time associated with an actvity
// such as an RPC call, DB query, or method invocation.
type Span interface {
	// BeginSpan starts a new Span, returning a child of this Span.
	BeginSpan(spanName string, args ...interface{}) Span
	// BeginProfile starts a new Profile, used to measure a named span
	// of time spent in this Span.
	BeginProfile(profileName string, args ...interface{}) Profile
	// End ends a Span, optionally reporting KV pairs provided by args.
	End(args ...interface{})
	// Add additional KV pairs that will be serialized (and dereferenced, for pointer
	// values) at the end of this trace's span.
	AddEndArgs(args ...interface{})

	// Info reports KV pairs provided by args for this Span.
	Info(args ...interface{})
	// Error reports details about an error (along with a stack trace) for this Span.
	Error(class, msg string)
	// Err reports details about error err (along with a stack trace) for this Span.
	Err(error)

	// MetadataString returns a string representing this Span for use
	// in distributed tracing, e.g. to provide as an "X-Trace" header
	// in an outgoing HTTP request.
	MetadataString() string

	// IsSampled returns whether or not this Layer is sampled
	IsSampled() bool

	// SetAsync(true) provides a hint that this Span is a parent of
	// concurrent overlapping child Spans.
	SetAsync(bool)

	// SetTransactionName sets this service's transaction name.
	// It is used for categorizing service metrics and traces in AppOptics.
	SetTransactionName(string) error

	IsReporting() bool
	addChildEdge(reporter.Context)
	addProfile(Profile)
	aoContext() reporter.Context
	ok() bool
}

// Profile is used to provide micro-benchmarks of named timings inside a Span.
type Profile interface {
	// End ends a Profile, optionally reporting KV pairs provided by args.
	End(args ...interface{})
	// Error reports details about an error (along with a stack trace) for this Profile.
	Error(class, msg string)
	// Err reports details about error err (along with a stack trace) for this Profile.
	Err(error)
}

// BeginSpan starts a new Span, provided a parent context and name. It returns a Span
// and context bound to the new child Span.
func BeginSpan(ctx context.Context, spanName string, args ...interface{}) (Span, context.Context) {
	if parent, ok := fromContext(ctx); ok && parent.ok() { // report span entry from parent context
		l := newSpan(parent.aoContext().Copy(), spanName, parent, args...)
		return l, newSpanContext(ctx, l)
	}
	return nullSpan{}, ctx
}

// BeginSpan starts a new Span, returning a child of this Span.
func (s *layerSpan) BeginSpan(spanName string, args ...interface{}) Span {
	if s.ok() { // copy parent context and report entry from child
		return newSpan(s.aoCtx.Copy(), spanName, s, args...)
	}
	return nullSpan{}
}

// BeginProfile begins a profiled block or method and return a context that should be closed with End().
// You can use defer to profile a function in one line, as below:
//   func exampleFunc(ctx context.Context) {
//       defer ao.BeginProfile(ctx, "exampleFunc").End()
//       // ... do something ...
//    }
func BeginProfile(ctx context.Context, profileName string, args ...interface{}) Profile {
	if parent, ok := fromContext(ctx); ok && parent.ok() { // report profile entry from parent context
		return newProfile(parent.aoContext().Copy(), profileName, parent, args...)
	}
	return nullSpan{}
}

// BeginProfile starts a new Profile, used to measure a named span of time spent in this Span.
// The returned Profile should be closed with End().
func (s *layerSpan) BeginProfile(profileName string, args ...interface{}) Profile {
	if s.ok() { // copy parent context and report entry from child
		return newProfile(s.aoCtx.Copy(), profileName, s, args...)
	}
	return nullSpan{}
}

// End a profiled block or method.
func (s *span) End(args ...interface{}) {
	if s.ok() {
		s.lock.Lock()
		defer s.lock.Unlock()
		for _, prof := range s.childProfiles {
			prof.End()
		}
		args = append(args, s.endArgs...)
		for _, edge := range s.childEdges { // add Edge KV for each joined child
			args = append(args, "Edge", edge)
		}
		_ = s.aoCtx.ReportEvent(s.exitLabel(), s.layerName(), args...)
		s.childEdges = nil // clear child edge list
		s.endArgs = nil
		s.ended = true
		// add this span's context to list to be used as Edge by parent exit
		if s.parent != nil && s.parent.ok() {
			s.parent.addChildEdge(s.aoCtx)
		}
	}
}

// Add KV pairs as variadic args that will be serialized (and dereferenced, for pointer
// values) at the end of this trace's span.
func (s *layerSpan) AddEndArgs(args ...interface{}) {
	if s.ok() {
		// ensure even number of args added
		if len(args)%2 == 1 {
			args = args[0 : len(args)-1]
		}
		s.lock.Lock()
		s.endArgs = append(s.endArgs, args...)
		s.lock.Unlock()
	}
}

// Info reports KV pairs provided by args.
func (s *layerSpan) Info(args ...interface{}) {
	if s.ok() {
		_ = s.aoCtx.ReportEvent(reporter.LabelInfo, s.layerName(), args...)
	}
}

// MetadataString returns a representation of the Span's context for use with distributed
// tracing (to create a remote child span). If the Span has ended, an empty string is returned.
func (s *layerSpan) MetadataString() string {
	if s.ok() {
		return s.aoCtx.MetadataString()
	}
	return ""
}

func (s *layerSpan) IsSampled() bool {
	if s.ok() {
		return s.aoCtx.IsSampled()
	}
	return false
}

// SetAsync provides a hint that this Span is a parent of concurrent overlapping child Spans.
func (s *layerSpan) SetAsync(val bool) {
	if val {
		s.AddEndArgs("Async", true)
	}
}

// SetTransactionName sets the transaction name used to categorize service requests in AppOptics.
func (s *span) SetTransactionName(name string) error {
	if !s.ok() {
		return errors.New("failed to set custom transaction name, invalid span")
	}
	if name == "" || len(name) > MaxCustomTransactionNameLength {
		return errors.New("valid length for custom transaction name: 1~255")
	}
	s.aoCtx.SetTransactionName(name)
	return nil
}

// Error reports an error, distinguished by its class and message
func (s *span) Error(class, msg string) {
	if s.ok() {
		s.aoCtx.ReportEvent(reporter.LabelError, s.layerName(),
			"ErrorClass", class, "ErrorMsg", msg, "Backtrace", debug.Stack())
	}
}

// Err reports the provided error type
func (s *span) Err(err error) {
	if s.ok() && err != nil {
		s.aoCtx.ReportEvent(reporter.LabelError, s.layerName(),
			"ErrorClass", "error", "ErrorMsg", err.Error(), "Backtrace", debug.Stack())
	}
}

// span satisfies the Extent interface and consolidates common reporting routines used by
// both Span and Profile interfaces.
type span struct {
	labeler
	aoCtx         reporter.Context
	parent        Span
	childEdges    []reporter.Context // for reporting in exit event
	childProfiles []Profile
	endArgs       []interface{}
	ended         bool // has exit event been reported?
	lock          sync.RWMutex
}
type layerSpan struct{ span }   // satisfies Span
type profileSpan struct{ span } // satisfies Profile
type nullSpan struct{}          // a span that is not tracing; satisfies Span & Profile

func (s nullSpan) BeginSpan(spanName string, args ...interface{}) Span   { return nullSpan{} }
func (s nullSpan) BeginProfile(name string, args ...interface{}) Profile { return nullSpan{} }
func (s nullSpan) End(args ...interface{})                               {}
func (s nullSpan) AddEndArgs(args ...interface{})                        {}
func (s nullSpan) Error(class, msg string)                               {}
func (s nullSpan) Err(err error)                                         {}
func (s nullSpan) Info(args ...interface{})                              {}
func (s nullSpan) IsReporting() bool                                     { return false }
func (s nullSpan) addChildEdge(reporter.Context)                         {}
func (s nullSpan) addProfile(Profile)                                    {}
func (s nullSpan) ok() bool                                              { return false }
func (s nullSpan) aoContext() reporter.Context                           { return reporter.NewNullContext() }
func (s nullSpan) MetadataString() string                                { return "" }
func (s nullSpan) IsSampled() bool                                       { return false }
func (s nullSpan) SetAsync(bool)                                         {}
func (s nullSpan) SetTransactionName(string) error                       { return nil }

// is this span still valid (has it timed out, expired, not sampled)
func (s *span) ok() bool {
	if s == nil {
		return false
	}
	s.lock.RLock()
	defer s.lock.RUnlock()
	return !s.ended
}
func (s *span) IsReporting() bool           { return s.ok() }
func (s *span) aoContext() reporter.Context { return s.aoCtx }

// addChildEdge keeps track of edges to closed child spans
func (s *span) addChildEdge(ctx reporter.Context) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.childEdges = append(s.childEdges, ctx)
}
func (s *span) addProfile(p Profile) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.childProfiles = append([]Profile{p}, s.childProfiles...)
}

// labelers help spans choose label and layer names.
type labeler interface {
	entryLabel() reporter.Label
	exitLabel() reporter.Label
	layerName() string
}
type spanLabeler struct{ name string }
type profileLabeler struct{ name string }

// AO's Span and Profile spans report their layer and label names slightly differently
func (l spanLabeler) entryLabel() reporter.Label    { return reporter.LabelEntry }
func (l spanLabeler) exitLabel() reporter.Label     { return reporter.LabelExit }
func (l spanLabeler) layerName() string             { return l.name }
func (l profileLabeler) entryLabel() reporter.Label { return reporter.LabelProfileEntry }
func (l profileLabeler) exitLabel() reporter.Label  { return reporter.LabelProfileExit }
func (l profileLabeler) layerName() string          { return "" }

func newSpan(aoCtx reporter.Context, spanName string, parent Span, args ...interface{}) Span {
	ll := spanLabeler{spanName}
	if err := aoCtx.ReportEvent(ll.entryLabel(), ll.layerName(), args...); err != nil {
		return nullSpan{}
	}
	return &layerSpan{span: span{aoCtx: aoCtx.Copy(), labeler: ll, parent: parent}}

}

func newProfile(aoCtx reporter.Context, profileName string, parent Span, args ...interface{}) Profile {
	var fname string
	pc, file, line, ok := runtime.Caller(2) // Caller(1) is BeginProfile
	if ok {
		f := runtime.FuncForPC(pc)
		fname = f.Name()
	}
	pl := profileLabeler{profileName}
	if err := aoCtx.ReportEvent(pl.entryLabel(), pl.layerName(), // report profile entry
		"Language", "go", "ProfileName", profileName,
		"FunctionName", fname, "File", file, "LineNumber", line,
	); err != nil {
		return nullSpan{}
	}
	p := &profileSpan{span{aoCtx: aoCtx.Copy(), labeler: pl, parent: parent,
		endArgs: []interface{}{"Language", "go", "ProfileName", profileName}}}
	if parent != nil && parent.ok() {
		parent.addProfile(p)
	}
	return p
}
