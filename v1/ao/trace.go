// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
)

// Trace represents a distributed trace for this request that reports
// events to AppOptics.
type Trace interface {
	// Inherited from the Span interface
	//  BeginSpan(spanName string, args ...interface{}) Span
	//  BeginProfile(profileName string, args ...interface{}) Profile
	//	End(args ...interface{})
	//	Info(args ...interface{})
	//  Error(class, msg string)
	//  Err(error)
	//  IsSampled() bool
	Span

	// End a Trace, and include KV pairs returned by func f. Useful
	// alternative to End() when used with defer to delay evaluation
	// of KVs until the end of the trace (since a deferred function's
	// arguments are evaluated when the defer statement is
	// evaluated). Func f will not be called at all if this span is
	// not tracing.
	EndCallback(f func() KVMap)

	// ExitMetadata returns a hex string that propagates the end of
	// this span back to a remote client. It is typically used in an
	// response header (e.g. the HTTP Header "X-Trace"). Call this
	// method to set a response header in advance of calling End().
	ExitMetadata() string

	// SetStartTime sets the start time of a trace.
	SetStartTime(start time.Time)

	// SetMethod sets the request's HTTP method of the trace, if any
	SetMethod(method string)

	// SetControllerAction sets controller/action for the trace
	SetControllerAction(controller, action string)
}

// KVMap is a map of additional key-value pairs to report along with the event data provided
// to AppOptics. Certain key names (such as "Query" or "RemoteHost") are used by AppOptics to
// provide details about program activity and distinguish between different types of spans.
// Please visit https://docs.appoptics.com/kb/apm_tracing/custom_instrumentation/ for
// details on the key names that AppOptics looks for.
type KVMap map[string]interface{}

type traceHTTPSpan struct {
	span       reporter.HttpSpanMessage
	start      time.Time
	controller string
	action     string
}

type aoTrace struct {
	layerSpan
	exitEvent reporter.Event
	httpSpan  traceHTTPSpan
}

func (t *aoTrace) aoContext() reporter.Context { return t.aoCtx }

// NewTrace creates a new Trace for reporting to AppOptics and immediately records
// the beginning of a root span named spanName. If this trace is sampled, it may report
// event data to AppOptics; otherwise event reporting will be a no-op.
func NewTrace(spanName string) Trace {
	ctx, ok := reporter.NewContext(spanName, "", true, nil)
	if !ok {
		return &nullTrace{}
	}
	return &aoTrace{
		layerSpan: layerSpan{span: span{aoCtx: ctx, labeler: spanLabeler{spanName}}},
	}
}

// NewTraceFromID creates a new Trace for reporting to AppOptics, provided an
// incoming trace ID (e.g. from a incoming RPC or service call's "X-Trace" header).
// If callback is provided & trace is sampled, cb will be called for entry event KVs
func NewTraceFromID(spanName, mdstr string, cb func() KVMap) Trace {
	ctx, ok := reporter.NewContext(spanName, mdstr, true, func() map[string]interface{} {
		if cb != nil {
			return cb()
		}
		return nil
	})
	if !ok {
		return &nullTrace{}
	}
	return &aoTrace{
		layerSpan: layerSpan{span: span{aoCtx: ctx, labeler: spanLabeler{spanName}}},
	}
}

// EndTrace reports the exit event for the span name that was used when calling NewTrace().
// No more events should be reported from this trace.
func (t *aoTrace) End(args ...interface{}) {
	if t.ok() {
		t.AddEndArgs(args...)
		t.reportExit()
	}
}

// EndCallback ends a Trace, reporting additional KV pairs returned by calling cb
func (t *aoTrace) EndCallback(cb func() KVMap) {
	if t.ok() {
		if cb != nil {
			var args []interface{}
			for k, v := range cb() {
				args = append(args, k, v)
			}
			t.AddEndArgs(args...)
		}
		t.reportExit()
	}
}

// SetStartTime sets the start time of a trace
func (t *aoTrace) SetStartTime(start time.Time) {
	t.httpSpan.start = start
}

// SetMethod sets the request's HTTP method, if any
func (t *aoTrace) SetMethod(method string) {
	t.httpSpan.span.Method = method
}

// SetControllerAction sets the controller and action
func (t *aoTrace) SetControllerAction(controller, action string) {
	t.httpSpan.controller = controller
	t.httpSpan.action = action
}

func (t *aoTrace) reportExit() {
	if t.ok() {
		t.lock.Lock()
		defer t.lock.Unlock()

		// if this is an HTTP trace, record a new span
		if !t.httpSpan.start.IsZero() {
			t.httpSpan.span.Duration = time.Now().Sub(t.httpSpan.start)
			t.recordHTTPSpan()
		}

		for _, edge := range t.childEdges { // add Edge KV for each joined child
			t.endArgs = append(t.endArgs, "Edge", edge)
		}
		if t.exitEvent != nil { // use exit event, if one was provided
			_ = t.exitEvent.ReportContext(t.aoCtx, true, t.endArgs...)
		} else {
			_ = t.aoCtx.ReportEvent(reporter.LabelExit, t.layerName(), t.endArgs...)
		}

		t.childEdges = nil // clear child edge list
		t.endArgs = nil
		t.ended = true
	}
}

func (t *aoTrace) IsSampled() bool { return t != nil && t.aoCtx.IsSampled() }

// ExitMetadata reports the X-Trace metadata string that will be used by the exit event.
// This is useful for setting response headers before reporting the end of the span.
func (t *aoTrace) ExitMetadata() (mdHex string) {
	if t.IsSampled() {
		if t.exitEvent == nil {
			t.exitEvent = t.aoCtx.NewEvent(reporter.LabelExit, t.layerName(), false)
		}
		if t.exitEvent != nil {
			mdHex = t.exitEvent.MetadataString()
		}
	}
	return
}

// recordHTTPSpan extract http status, controller and action from the deferred endArgs
// and fill them into trace's httpSpan struct. The data is then sent to the span message channel.
func (t *aoTrace) recordHTTPSpan() {
	var controller, action string
	num := len([]string{"Status", "Controller", "Action"})
	for i := 0; (i+1 < len(t.endArgs)) && (num > 0); i += 2 {
		k, isStr := t.endArgs[i].(string)
		if !isStr {
			continue
		}
		if k == "Status" {
			t.httpSpan.span.Status = *(t.endArgs[i+1].(*int))
			num--
		} else if k == "Controller" {
			controller += t.endArgs[i+1].(string)
			num--
		} else if k == "Action" {
			action += t.endArgs[i+1].(string)
			num--
		}
	}

	if t.httpSpan.controller != "" && t.httpSpan.action != "" {
		t.httpSpan.span.Transaction = t.httpSpan.controller + "." + t.httpSpan.action
	} else if controller != "" && action != "" {
		t.httpSpan.span.Transaction = controller + "." + action
	}

	if t.httpSpan.span.Status >= 500 && t.httpSpan.span.Status < 600 {
		t.httpSpan.span.HasError = true
	}

	reporter.ReportSpan(&t.httpSpan.span)
}

// A nullTrace is not tracing.
type nullTrace struct{ nullSpan }

func (t *nullTrace) EndCallback(f func() KVMap)                    {}
func (t *nullTrace) ExitMetadata() string                          { return "" }
func (t *nullTrace) SetStartTime(start time.Time)                  {}
func (t *nullTrace) SetMethod(method string)                       {}
func (t *nullTrace) SetControllerAction(controller, action string) {}
func (t *nullTrace) recordMetrics()                                {}

// NewNullTrace returns a trace that is not sampled.
func NewNullTrace() Trace { return &nullTrace{} }
