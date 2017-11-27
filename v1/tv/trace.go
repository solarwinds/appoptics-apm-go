// Copyright (C) 2016 Librato, Inc. All rights reserved.

package tv

import (
	"strings"
	"time"

	"github.com/librato/go-traceview/v1/tv/internal/traceview"
)

// Trace represents a distributed trace for this request that reports
// events to TraceView.
type Trace interface {
	// Inherited from the Layer interface
	//  BeginLayer(layerName string, args ...interface{}) Layer
	//  BeginProfile(profileName string, args ...interface{}) Profile
	//	End(args ...interface{})
	//	Info(args ...interface{})
	//  Error(class, msg string)
	//  Err(error)
	//  IsSampled() bool
	Layer

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
}

// KVMap is a map of additional key-value pairs to report along with the event data provided
// to TraceView. Certain key names (such as "Query" or "RemoteHost") are used by TraceView to
// provide details about program activity and distinguish between different types of layers.
// Please visit http://docs.appneta.com/traceview-instrumentation#special-interpretation for
// details on the key names that TraceView looks for.
type KVMap map[string]interface{}

type traceHttpSpan struct {
	span  traceview.HttpSpanMessage
	start time.Time
}

type tvTrace struct {
	layerSpan
	exitEvent traceview.Event
	httpSpan  traceHttpSpan
}

func (t *tvTrace) tvContext() traceview.Context { return t.tvCtx }

// NewTrace creates a new Trace for reporting to TraceView and immediately records
// the beginning of the layer layerName. If this trace is sampled, it may report
// event data to TraceView; otherwise event reporting will be a no-op.
func NewTrace(layerName string) Trace {
	ctx, ok := traceview.NewContext(layerName, "", true, nil)
	if !ok {
		return &nullTrace{}
	}
	return &tvTrace{
		layerSpan: layerSpan{span: span{tvCtx: ctx, labeler: layerLabeler{layerName}}},
	}
}

// NewTraceFromID creates a new Trace for reporting to TraceView, provided an
// incoming trace ID (e.g. from a incoming RPC or service call's "X-Trace" header).
// If callback is provided & trace is sampled, cb will be called for entry event KVs
func NewTraceFromID(layerName, mdstr string, cb func() KVMap) Trace {
	ctx, ok := traceview.NewContext(layerName, mdstr, true, func() map[string]interface{} {
		if cb != nil {
			return cb()
		}
		return nil
	})
	if !ok {
		return &nullTrace{}
	}
	return &tvTrace{
		layerSpan: layerSpan{span: span{tvCtx: ctx, labeler: layerLabeler{layerName}}},
	}
}

// EndTrace reports the exit event for the layer name that was used when calling NewTrace().
// No more events should be reported from this trace.
func (t *tvTrace) End(args ...interface{}) {
	if t.ok() {
		t.AddEndArgs(args...)
		t.reportExit()
	}
}

// EndCallback ends a Trace, reporting additional KV pairs returned by calling cb
func (t *tvTrace) EndCallback(cb func() KVMap) {
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
func (t *tvTrace) SetStartTime(start time.Time) {
	t.httpSpan.start = start
}

// SetMethod sets the request's HTTP method, if any
func (t *tvTrace) SetMethod(method string) {
	t.httpSpan.span.Method = method
}

func (t *tvTrace) reportExit() {
	if t.ok() {
		t.lock.Lock()
		defer t.lock.Unlock()
		for _, edge := range t.childEdges { // add Edge KV for each joined child
			t.endArgs = append(t.endArgs, "Edge", edge)
		}
		if t.exitEvent != nil { // use exit event, if one was provided
			_ = t.exitEvent.ReportContext(t.tvCtx, true, t.endArgs...)
		} else {
			_ = t.tvCtx.ReportEvent(traceview.LabelExit, t.layerName(), t.endArgs...)
		}
		t.recordHTTPSpan()
		t.childEdges = nil // clear child edge list
		t.endArgs = nil
		t.ended = true
	}
}

func (t *tvTrace) IsSampled() bool { return t != nil && t.tvCtx.IsSampled() }

// ExitMetadata reports the X-Trace metadata string that will be used by the exit event.
// This is useful for setting response headers before reporting the end of the span.
func (t *tvTrace) ExitMetadata() (mdHex string) {
	if t.IsSampled() {
		if t.exitEvent == nil {
			t.exitEvent = t.tvCtx.NewEvent(traceview.LabelExit, t.layerName(), false)
		}
		if t.exitEvent != nil {
			mdHex = t.exitEvent.MetadataString()
		}
	}
	return
}

// recordHTTPSpan extract http status, controller and action from the deferred endArgs
// and fill them into trace's httpSpan struct. The data is then sent to the span message channel.
func (t *tvTrace) recordHTTPSpan() {
	var transaction string
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
			transaction += t.endArgs[i+1].(string) + "."
			num--
		} else if k == "Action" {
			transaction += t.endArgs[i+1].(string) + "."
			num--
		}
	}

	t.httpSpan.span.Transaction = strings.TrimSuffix(transaction, ".")
	t.httpSpan.span.Duration = time.Now().Sub(t.httpSpan.start)

	traceview.ReportSpan(&t.httpSpan.span)
}

// A nullTrace is not tracing.
type nullTrace struct{ nullSpan }

func (t *nullTrace) EndCallback(f func() KVMap)   {}
func (t *nullTrace) ExitMetadata() string         { return "" }
func (t *nullTrace) SetStartTime(start time.Time) {}
func (t *nullTrace) SetMethod(method string)      {}
func (t *nullTrace) recordMetrics()               {}

// NewNullTrace returns a trace that is not sampled.
func NewNullTrace() Trace { return &nullTrace{} }
