// Copyright (C) 2016 Librato, Inc. All rights reserved.
// TraceView HTTP instrumentation for Go

package tv

import (
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"time"
)

// HTTPHeaderName is a constant for the HTTP header used by TraceView ("X-Trace") to propagate
// the distributed tracing context across HTTP requests.
const HTTPHeaderName = "X-Trace"
const httpHandlerLayerName = "http.HandlerFunc"

// HTTPHandler wraps an http.HandlerFunc with entry / exit events,
// returning a new handler that can be used in its place.
//   http.HandleFunc("/path", tv.HTTPHandler(myHandler))
func HTTPHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	// At wrap time (when binding handler to router): get name of wrapped handler func
	var endArgs []interface{}
	if f := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()); f != nil {
		// e.g. "main.slowHandler", "github.com/librato/go-traceview/v1/tv_test.handler404"
		fname := f.Name()
		if s := strings.SplitN(fname[strings.LastIndex(fname, "/")+1:], ".", 2); len(s) == 2 {
			endArgs = append(endArgs, "Controller", s[0], "Action", s[1])
		}
	}
	// return wrapped HTTP request handler
	return func(w http.ResponseWriter, r *http.Request) {
		t, w := TraceFromHTTPRequestResponse(httpHandlerLayerName, w, r)
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

// TraceFromHTTPRequestResponse returns a Trace and a wrapped http.ResponseWriter, given a
// http.ResponseWriter and http.Request. If a distributed trace is described in the HTTP request
// headers, the trace's context will be continued. The returned http.ResponseWriter should be used
// in place of the one passed into this function in order to observe the response's headers and
// status code.
//   func myHandler(w http.ResponseWriter, r *http.Request) {
//       tr, w := tv.TraceFromHTTPRequestResponse("myHandler", w, r)
//       defer tr.End()
//       // ...
//   }
func TraceFromHTTPRequestResponse(layerName string, w http.ResponseWriter, r *http.Request) (Trace, http.ResponseWriter) {
	t := traceFromHTTPRequest(layerName, r)
	wrapper := newResponseWriter(w, t) // wrap writer with response-observing writer
	return t, wrapper
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
func (w *HTTPResponseWriter) Header() http.Header { return w.Writer.Header() }

func (w *HTTPResponseWriter) WriteHeader(status int) {
	w.StatusCode = status                // observe HTTP status code
	md := w.Header().Get(HTTPHeaderName) // check response for downstream metadata
	if w.t.IsTracing() {                 // set trace exit metadata in X-Trace header
		// if downstream response headers mention a different layer, add edge to it
		if md != "" && md != w.t.ExitMetadata() {
			w.t.AddEndArgs("Edge", md)
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
	t.AddEndArgs("Status", &w.StatusCode)
	// add exit event metadata to X-Trace header
	if t.IsTracing() {
		// add/replace response header metadata with this trace's
		w.Header().Set(HTTPHeaderName, t.ExitMetadata())
	}
	return w
}

// traceFromHTTPRequest returns a Trace, given an http.Request. If a distributed trace is described
// in the "X-Trace" header, this context will be continued.
func traceFromHTTPRequest(layerName string, r *http.Request) Trace {
	// start trace, passing in metadata header
	header := r.Header.Get(HTTPHeaderName)
	t := NewTraceFromID(layerName, header, func() KVMap {
		return KVMap{
			"Method":       r.Method,
			"HTTP-Host":    r.Host,
			"URL":          r.URL.Path,
			"Remote-Host":  r.RemoteAddr,
			"Query-String": r.URL.RawQuery,
		}
	})
	// set the start time and method for metrics collection
	t.SetMethod(r.Method)
	if header == "" {
		t.SetStartTime(time.Now())
	}
	// update incoming metadata in request headers for any downstream readers
	r.Header.Set(HTTPHeaderName, t.MetadataString())
	return t
}
