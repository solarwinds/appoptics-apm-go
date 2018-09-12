// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"strings"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"golang.org/x/net/context"
)

// Trace represents the root span of a distributed trace for this request that reports
// events to AppOptics. The Trace interface extends the Span interface with additional
// methods that can be used to help categorize a service's inbound requests on the
// AppOptics service dashboard.
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

	// SetMethod sets the request's HTTP method of the trace, if any.
	// It is used for categorizing service metrics and traces in AppOptics.
	SetMethod(method string)

	// SetPath extracts the full Path from http.Request
	SetPath(url string)

	// SetHost extracts the host information from http.Request
	SetHost(host string)

	// SetStatus sets the request's HTTP status code of the trace, if any.
	// It is used for categorizing service metrics and traces in AppOptics.
	SetStatus(status int)

	// SetStartTime sets the start time of a span.
	SetStartTime(start time.Time)
}

// KVMap is a map of additional key-value pairs to report along with the event data provided
// to AppOptics. Certain key names (such as "Query" or "RemoteHost") are used by AppOptics to
// provide details about program activity and distinguish between different types of spans.
// Please visit https://docs.appoptics.com/kb/apm_tracing/custom_instrumentation/ for
// details on the key names that AppOptics looks for.
type KVMap map[string]interface{}

type traceHTTPSpan struct {
	span       reporter.HTTPSpanMessage
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
	t := &aoTrace{
		layerSpan: layerSpan{span: span{aoCtx: ctx, labeler: spanLabeler{spanName}}},
	}
	t.SetStartTime(time.Now())
	return t
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
	t := &aoTrace{
		layerSpan: layerSpan{span: span{aoCtx: ctx, labeler: spanLabeler{spanName}}},
	}
	t.SetStartTime(time.Now())
	return t
}

// SetTransactionName can be called inside a http handler to set the custom transaction name.
func SetTransactionName(ctx context.Context, name string) error {
	return TraceFromContext(ctx).SetTransactionName(name)
}

// GetTransactionName fetches the current transaction name from the context
func GetTransactionName(ctx context.Context) string {
	return TraceFromContext(ctx).GetTransactionName()
}

// End reports the exit event for the span name that was used when calling NewTrace().
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

// SetPath extracts the Path from http.Request
func (t *aoTrace) SetPath(path string) {
	t.httpSpan.span.Path = path
}

// SetHost extracts the host information from http.Request
func (t *aoTrace) SetHost(host string) {
	t.httpSpan.span.Host = host
}

// SetStatus sets the request's HTTP status code of the trace, if any
func (t *aoTrace) SetStatus(status int) {
	t.httpSpan.span.Status = status
}

func (t *aoTrace) reportExit() {
	if t.ok() {
		t.lock.Lock()
		defer t.lock.Unlock()

		// The trace may have been ended by another goroutine (?) after the last
		// check (t.ok()) but before we acquire the lock. So a double check is
		// worthwhile.
		// However, we need to check t.ended directly as t.ok() will cause deadlock.
		if t.ended {
			return
		}

		// if this is an HTTP trace, record a new span
		if !t.httpSpan.start.IsZero() {
			t.httpSpan.span.Duration = time.Now().Sub(t.httpSpan.start)
			t.recordHTTPSpan()
		}

		for _, edge := range t.childEdges { // add Edge KV for each joined child
			t.endArgs = append(t.endArgs, "Edge", edge)
		}
		if t.exitEvent != nil { // use exit event, if one was provided
			t.exitEvent.ReportContext(t.aoCtx, true, t.endArgs...)
		} else {
			t.aoCtx.ReportEvent(reporter.LabelExit, t.layerName(), t.endArgs...)
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
			switch v := t.endArgs[i+1].(type) {
			case int:
				t.httpSpan.span.Status = v
			case *int:
				t.httpSpan.span.Status = *v
			}
			num--
		} else if k == "Controller" {
			controller += t.endArgs[i+1].(string)
			num--
		} else if k == "Action" {
			action += t.endArgs[i+1].(string)
			num--
		}
	}

	t.finalizeTxnName(controller, action)

	if t.httpSpan.span.Status >= 500 && t.httpSpan.span.Status < 600 {
		t.httpSpan.span.HasError = true
	}

	reporter.ReportSpan(&t.httpSpan.span)

	// This will add the TransactionName KV into the exit event.
	t.endArgs = append(t.endArgs, "TransactionName", t.httpSpan.span.Transaction)
}

// finalizeTxnName finalizes the transaction name based on the following factors:
// custom transaction name, action/controller, Path and the value of APPOPTICS_PREPEND_DOMAIN
func (t *aoTrace) finalizeTxnName(controller string, action string) {
	// The precedence:
	// custom transaction name > framework specific transaction naming > controller.action > 1st and 2nd segment of Path
	customTxnName := t.aoCtx.GetTransactionName()
	if customTxnName != "" {
		t.httpSpan.span.Transaction = customTxnName
	} else if t.httpSpan.controller != "" && t.httpSpan.action != "" {
		t.httpSpan.span.Transaction = t.httpSpan.controller + "." + t.httpSpan.action
	} else if controller != "" && action != "" {
		t.httpSpan.span.Transaction = controller + "." + action
	} else if t.httpSpan.span.Path != "" {
		t.httpSpan.span.Transaction = reporter.GetTransactionFromPath(t.httpSpan.span.Path)
	}

	if t.httpSpan.span.Transaction == "" {
		t.httpSpan.span.Transaction = reporter.UnknownTransactionName
	}
	t.prependDomainToTxnName()
}

// prependDomainToTxnName prepends the domain to the transaction name if APPOPTICS_PREPEND_DOMAIN = true
func (t *aoTrace) prependDomainToTxnName() {
	if !config.GetPrependDomain() ||
		t.httpSpan.span.Transaction == reporter.UnknownTransactionName ||
		t.httpSpan.span.Host == "" {
		return
	}
	if strings.HasSuffix(t.httpSpan.span.Host, "/") ||
		strings.HasPrefix(t.httpSpan.span.Transaction, "/") {
		t.httpSpan.span.Transaction = t.httpSpan.span.Host + t.httpSpan.span.Transaction
	} else {
		t.httpSpan.span.Transaction = t.httpSpan.span.Host + "/" + t.httpSpan.span.Transaction
	}
}

// A nullTrace is not tracing.
type nullTrace struct{ nullSpan }

func (t *nullTrace) EndCallback(f func() KVMap)   {}
func (t *nullTrace) ExitMetadata() string         { return "" }
func (t *nullTrace) SetStartTime(start time.Time) {}
func (t *nullTrace) SetMethod(method string)      {}
func (t *nullTrace) SetPath(path string)          {}
func (t *nullTrace) SetHost(host string)          {}
func (t *nullTrace) SetStatus(status int)         {}
func (t *nullTrace) recordMetrics()               {}

// NewNullTrace returns a trace that is not sampled.
func NewNullTrace() Trace { return &nullTrace{} }

// IsReady checks if the agent is ready. It will block until the agent is ready
// or the time is out.
func IsReady(timeout time.Duration) bool {
	return reporter.IsReady(timeout)
}

// Shutdown flush the metrics and stops the agent. It will block until the agent
// flushes and successfully shutdown or the time is out. It returns nil for successful
// shutdown and an error when timeout happens.
//
// This function should be called only once.
func Shutdown(wait time.Duration) error {
	return reporter.Shutdown(wait)
}
