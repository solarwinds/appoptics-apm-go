// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv

import "github.com/appneta/go-traceview/v1/tv/internal/traceview"

// Trace represents a distributed trace for this request that reports
// events to AppNeta TraceView.
type Trace interface {
	IsTracing() bool

	// Inherited from the Layer interface
	//  BeginLayer(layerName string, args ...interface{}) Layer
	//  BeginProfile(profileName string, args ...interface{}) Profile
	//	End(args ...interface{})
	//	Info(args ...interface{})
	//  Error(class, msg string)
	//  Err(error)
	Layer

	// End a trace, and include KV pairs returned by func f
	EndCallback(f func() KVMap)

	// Return metadata string for use e.g. in a HTTP header named "X-Trace"
	ExitMetadata() string
}

// KVMap is a map of additional key-value pairs to report along with the event data provided
// to TraceView. Certain key names (such as "Query" or "RemoteHost") are used by AppNeta to
// provide details about program activity and distinguish between different types of layers.
// Please visit http://docs.appneta.com/traceview-instrumentation#special-interpretation for
// details on the key names that TraceView looks for.
type KVMap map[string]interface{}

type tvTrace struct {
	layerSpan
	exitEvent traceview.SampledEvent
}

func (t *tvTrace) tvContext() traceview.SampledContext { return t.tvCtx }

// NewTrace creates a new trace for reporting to TraceView and immediately records
// the beginning of the layer layerName. If this trace is sampled, it may report
// event data to AppNeta; otherwise event reporting will be a no-op.
func NewTrace(layerName string) Trace {
	ctx := traceview.NewSampledContext(layerName, "", true, nil)
	lbl := layerLabeler{layerName}
	return &tvTrace{layerSpan{span: span{tvCtx: ctx, labeler: lbl}}, nil}
}

// NewTraceFromID creates a new trace for reporting to TraceView, provided an
// incoming trace ID (e.g. from a incoming RPC or service call's "X-Trace" header).
// If callback is provided & trace is sampled, cb will be called for entry event KVs
func NewTraceFromID(layerName, mdstr string, cb func() KVMap) Trace {
	ctx := traceview.NewSampledContext(layerName, mdstr, true, func() map[string]interface{} {
		if cb != nil {
			return cb()
		}
		return nil
	})
	lbl := layerLabeler{layerName}
	return &tvTrace{layerSpan{span: span{tvCtx: ctx, labeler: lbl}}, nil}
}

// EndTrace reports the exit event for the layer name that was used when calling NewTrace().
// No more events should be reported from this trace.
func (t *tvTrace) End(args ...interface{}) {
	if t != nil && !t.ended {
		for _, edge := range t.childEdges { // add Edge KV for each joined child
			args = append(args, "Edge", edge)
		}
		if t.exitEvent != nil { // use exit event, if one was provided
			t.exitEvent.ReportContext(t.tvCtx, true, args...)
		} else {
			t.tvCtx.ReportEvent(traceview.LabelExit, t.layerName(), args...)
		}
		t.childEdges = nil // clear child edge list
		t.ended = true
	}
}

// EndCallback ends a trace, reporting additional KV pairs returned by calling cb
func (t *tvTrace) EndCallback(cb func() KVMap) {
	if t != nil && !t.ended {
		var kvs map[string]interface{}
		if cb != nil {
			kvs = cb()
		}
		var args []interface{}
		for k, v := range kvs {
			args = append(args, k)
			args = append(args, v)
		}
		if t.exitEvent != nil { // use exit event, if one was provided
			t.exitEvent.ReportContext(t.tvCtx, true, args...)
		} else {
			t.tvCtx.ReportEvent(traceview.LabelExit, t.layerName(), args...)
		}
		t.ended = true
	}
}

func (t *tvTrace) IsTracing() bool { return t.tvCtx.IsTracing() }

// ExitMetadata reports the X-Trace metadata string that will be used by the exit event.
// This is useful for retrieving response headers in advance of reporting exit.
func (t *tvTrace) ExitMetadata() string {
	if t.IsTracing() {
		t.exitEvent = t.tvCtx.NewSampledEvent(traceview.LabelExit, t.layerName(), false)
		if t.exitEvent != nil {
			return t.exitEvent.MetadataString()
		}
	}
	return ""
}
