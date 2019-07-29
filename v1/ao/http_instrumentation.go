// +build go1.7
// Copyright (C) 2016 Librato, Inc. All rights reserved.
// AppOptics HTTP instrumentation for Go

package ao

import (
	"context"
	"fmt"
	"net/http"
	"net/textproto"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
)

const (
	// HTTPHeaderName is a constant for the HTTP header used by AppOptics ("X-Trace") to propagate
	// the distributed tracing context across HTTP requests.
	HTTPHeaderName                  = "X-Trace"
	HTTPHeaderXTraceOptions         = reporter.HTTPHeaderXTraceOptions
	HTTPHeaderXTraceOptionsResponse = reporter.HTTPHeaderXTraceOptionsResponse
	httpHandlerSpanName             = "http.HandlerFunc"
)

// key used for HTTP span to indicate a new context
var httpSpanKey = contextKeyT("github.com/appoptics/appoptics-apm-go/v1/ao.HTTPSpan")

// HTTPHandler wraps an http.HandlerFunc with entry / exit events,
// returning a new handler that can be used in its place.
//   http.HandleFunc("/path", ao.HTTPHandler(myHandler))
func HTTPHandler(handler func(http.ResponseWriter, *http.Request), opts ...SpanOpt) func(http.ResponseWriter, *http.Request) {
	if Disabled() {
		return handler
	}
	// At wrap time (when binding handler to router): get name of wrapped handler func
	var endArgs []interface{}
	if f := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()); f != nil {
		// e.g. "main.slowHandler", "github.com/appoptics/appoptics-apm-go/v1/ao_test.handler404"
		fname := f.Name()
		if s := strings.SplitN(fname[strings.LastIndex(fname, "/")+1:], ".", 2); len(s) == 2 {
			endArgs = append(endArgs, "Controller", s[0], "Action", s[1])
		}
	}
	// return wrapped HTTP request handler
	return func(w http.ResponseWriter, r *http.Request) {
		if Closed() {
			handler(w, r)
			return
		}

		t, w, r := TraceFromHTTPRequestResponse(httpHandlerSpanName, w, r, opts...)
		defer t.End(endArgs...)

		defer func() { // catch and report panic, if one occurs
			if err := recover(); err != nil {
				t.Error("panic", fmt.Sprintf("%v", err))
				panic(err) // re-raise the panic
			}
		}()
		// Call original HTTP handler
		handler(w, r)
	}
}

// TraceFromHTTPRequestResponse returns a Trace, a wrapped http.ResponseWriter, and a modified
// http.Request, given a http.ResponseWriter and http.Request. If a distributed trace is described
// in the HTTP request headers, the trace's context will be continued. The returned http.ResponseWriter
// should be used in place of the one passed into this function in order to observe the response's
// headers and status code.
//   func myHandler(w http.ResponseWriter, r *http.Request) {
//       tr, w, r := ao.TraceFromHTTPRequestResponse("myHandler", w, r)
//       defer tr.End()
//       // ...
//   }
func TraceFromHTTPRequestResponse(spanName string, w http.ResponseWriter, r *http.Request, opts ...SpanOpt) (Trace, http.ResponseWriter,
	*http.Request) {

	// determine if this is a new context, if so set flag isNewContext to start a new HTTP Span
	isNewContext := false
	if b, ok := r.Context().Value(httpSpanKey).(bool); !ok || !b {
		// save KV to ensure future calls won't treat as new context
		r = r.WithContext(context.WithValue(r.Context(), httpSpanKey, true))
		isNewContext = true
	}

	t := traceFromHTTPRequest(spanName, r, isNewContext, opts...)

	// Associate the trace with http.Request to expose it to the handler
	r = r.WithContext(NewContext(r.Context(), t))

	wrapper := newResponseWriter(w, t) // wrap writer with response-observing writer
	for k, v := range t.HTTPRspHeaders() {
		wrapper.Header().Set(k, v)
	}

	return t, wrapper, r
}

// HTTPResponseWriter observes an http.ResponseWriter when WriteHeader() or Write() is called to
// check the status code and response headers.
type HTTPResponseWriter struct {
	Writer      http.ResponseWriter
	t           Trace
	StatusCode  int
	WroteHeader bool
}

func (w *HTTPResponseWriter) Write(p []byte) (n int, err error) {
	if !w.WroteHeader {
		w.WriteHeader(w.StatusCode)
	}
	return w.Writer.Write(p)
}

// Header implements the http.ResponseWriter interface.
func (w *HTTPResponseWriter) Header() http.Header { return w.Writer.Header() }

// WriteHeader implements the http.ResponseWriter interface.
func (w *HTTPResponseWriter) WriteHeader(status int) {
	w.StatusCode = status                // observe HTTP status code
	md := w.Header().Get(HTTPHeaderName) // check response for downstream metadata
	if w.t.IsReporting() {               // set trace exit metadata in X-Trace header
		// if downstream response headers mention a different span, add edge to it
		if md != "" && md != w.t.ExitMetadata() {
			w.t.AddEndArgs(keyEdge, md)
		}
		w.Header().Set(HTTPHeaderName, w.t.ExitMetadata()) // replace downstream MD with ours
	}
	w.WroteHeader = true
	w.Writer.WriteHeader(status)
}

// newResponseWriter observes the HTTP Status code of an HTTP response, returning a
// wrapped http.ResponseWriter and a pointer to an int containing the status.
func newResponseWriter(writer http.ResponseWriter, t Trace) *HTTPResponseWriter {
	w := &HTTPResponseWriter{Writer: writer, t: t, StatusCode: http.StatusOK}
	t.AddEndArgs(keyStatus, &w.StatusCode)
	// add exit event metadata to X-Trace header
	if t.IsReporting() {
		// add/replace response header metadata with this trace's
		w.Header().Set(HTTPHeaderName, t.ExitMetadata())
	}
	return w
}

// CheckTriggerTraceHeader extracts the X-Trace-Options from the header
func CheckTriggerTraceHeader(header map[string][]string) (bool, map[string]string, []string) {
	var forceTrace bool
	var kvs map[string]string
	var ignoredKeys []string

	xTraceOptsSlice, ok := header[textproto.CanonicalMIMEHeaderKey(HTTPHeaderXTraceOptions)]
	if !ok {
		return false, kvs, ignoredKeys
	}

	kvs = make(map[string]string)

	xTraceOpts := strings.Split(strings.Join(xTraceOptsSlice, ","), ";")
	for _, opt := range xTraceOpts {
		kvSlice := strings.SplitN(opt, "=", 2)
		var k, v string
		k = strings.TrimSpace(kvSlice[0])
		if len(kvSlice) == 2 {
			v = strings.TrimSpace(kvSlice[1])
		}

		if !(strings.HasPrefix(strings.ToLower(k), "custom_") ||
			k == "pd_keys" ||
			k == "trigger_trace") {
			ignoredKeys = append(ignoredKeys, k)
			continue
		}
		if k == "pd_keys" {
			k = "PDKeys"
		}
		kvs[k] = v
	}
	val, forceTrace := kvs["trigger_trace"]
	if val != "" {
		log.Debug("trigger_trace should not contain any value.")
		forceTrace = false
		ignoredKeys = append(ignoredKeys, "trigger_trace")
	}
	delete(kvs, "trigger_trace")

	return forceTrace, kvs, ignoredKeys
}

// traceFromHTTPRequest returns a Trace, given an http.Request. If a distributed trace is described
// in the "X-Trace" header, this context will be continued.
func traceFromHTTPRequest(spanName string, r *http.Request, isNewContext bool, opts ...SpanOpt) Trace {
	so := &SpanOptions{}
	for _, f := range opts {
		f(so)
	}

	triggerTrace, triggerTraceKVs, ignoredKeys := CheckTriggerTraceHeader(r.Header)

	// start trace, passing in metadata header
	t := NewTraceWithOptions(spanName, SpanOptions{
		false,
		reporter.ContextOptions{
			MdStr:         r.Header.Get(HTTPHeaderName),
			URL:           r.URL.EscapedPath(),
			XTraceOptions: r.Header.Get(HTTPHeaderXTraceOptions) != "",
			TriggerTrace:  triggerTrace,
			CB: func() KVMap {
				kvs := KVMap{
					keyMethod:      r.Method,
					keyHTTPHost:    r.Host,
					keyURL:         r.URL.EscapedPath(),
					keyRemoteHost:  r.RemoteAddr,
					keyQueryString: r.URL.RawQuery,
				}

				for k, v := range triggerTraceKVs {
					kvs[k] = v
				}

				if so.WithBackTrace {
					kvs[KeyBackTrace] = string(debug.Stack())
				}

				return kvs
			}},
	})

	// set the start time and method for metrics collection
	t.SetMethod(r.Method)
	t.SetPath(r.URL.EscapedPath())

	var host string
	if host = r.Header.Get("X-Forwarded-Host"); host == "" {
		host = r.Host
	}
	t.SetHost(host)

	// Clear the start time if it is not a new context
	if !isNewContext {
		t.SetStartTime(time.Time{})
	}

	if len(ignoredKeys) != 0 {
		headers := t.HTTPRspHeaders()
		xTraceOptsRsp, ok := headers[HTTPHeaderXTraceOptionsResponse]
		ignored := "ignored=" + strings.Join(ignoredKeys, ",")
		if ok && xTraceOptsRsp != "" {
			xTraceOptsRsp = xTraceOptsRsp + ";" + ignored
		} else {
			xTraceOptsRsp = ignored
		}
		headers[HTTPHeaderXTraceOptionsResponse] = xTraceOptsRsp
		t.SetHTTPRspHeaders(headers)
	}
	// update incoming metadata in request headers for any downstream readers
	r.Header.Set(HTTPHeaderName, t.MetadataString())
	return t
}
