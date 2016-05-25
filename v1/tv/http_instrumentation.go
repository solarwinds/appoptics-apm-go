// Copyright (C) 2016 AppNeta, Inc. All rights reserved.
// TraceView HTTP instrumentation for Go

package tv

import (
	"log"
	"net/http"
	"reflect"
	"runtime"
	"strings"
)

var httpLayerName = "http.HandlerFunc"

// HTTPHandler wraps an http handler function with entry / exit events,
// returning a new function that can be used in its place.
func HTTPHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	// At wrap time (when binding handler to router): get name of wrapped handler func
	var controller, action string
	if f := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()); f != nil {
		// e.g. "main.slowHandler", "github.com/appneta/go-appneta/v1/tv_test.handler404"
		fname := f.Name()
		if s := strings.Split(fname[strings.LastIndex(fname, "/")+1:], "."); len(s) == 2 {
			controller = s[0]
			action = s[1]
		}
	}
	// return wrapped HTTP request handler
	return func(w http.ResponseWriter, r *http.Request) {
		t := TraceFromHTTPRequest(httpLayerName, r)
		// wrap writer with status-observing writer
		writer := &httpResponseWriter{w, t, http.StatusOK, ""}
		w = writer

		// add exit event's X-Trace header:
		var endArgs []interface{}
		if t.IsTracing() {
			// add/replace metadata header with this trace's exit
			w.Header().Set("X-Trace", t.ExitMetadata())
		}

		// Call original HTTP handler:
		handler(w, r)
		// _, wrapper := TraceFromHTTPRequestResponse(httpLayerName, w, r,
		// "Controller", controller, "Action", action)
		// handler(wrapper, r) // call original HTTP handler

		// check response headers for downstream metadata
		//		if md := w.Header().Get("X-Trace"); w.md != "" {
		if writer.metadata != "" {
			endArgs = append(endArgs, "Edge", writer.metadata)
		}
		// Add status code and report exit event
		endArgs = append(endArgs, "Status", writer.Status, "Controller", controller, "Action", action)
		log.Printf("endArgs %v\n", endArgs)
		t.End(endArgs...)

	}
}

func TraceFromHTTPRequestResponse(layerName string, w http.ResponseWriter, r *http.Request, args ...interface{}) (Trace, http.ResponseWriter) {
	t := TraceFromHTTPRequest(layerName, r)
	wrapper := NewResponseWriter(w, t)            // wrap writer with response-observing writer
	args = append(args, "Status", wrapper.Status) // add status code and report exit event
	defer t.End(args...)                          // report exit event
	return t, wrapper
}

// httpResponseWriter observes an http.ResponseWriter when WriteHeader is called to check
// the status code and response headers.
type httpResponseWriter struct {
	http.ResponseWriter
	t        Trace
	Status   int
	metadata string
}

func (w *httpResponseWriter) Write(buf []byte) (int, error) {
	log.Printf("Write len(buf) %v", len(buf))
	return w.ResponseWriter.Write(buf)
}

func (w *httpResponseWriter) WriteHeader(status int) {
	log.Printf("WriteHeader status %v", status)
	// observe HTTP status code
	w.Status = status
	var endArgs []interface{}
	// check response for downstream metadata
	w.metadata = w.Header().Get("X-Trace")
	//	if md := w.Header().Get("X-Trace"); md != "" {
	//		endArgs = append(endArgs, "Edge", md)
	//	}
	endArgs = append(endArgs, "Status", w.Status) // add status code and report exit event
	if w.t.IsTracing() {
		// set trace exit metadata in X-Trace header
		w.Header().Set("X-Trace", w.t.ExitMetadata())
	}
	// report exit event
	//w.t.End(endArgs...)
	w.ResponseWriter.WriteHeader(status)
}

// NewResponseWriter observes the HTTP Status code of an HTTP response, returning a
// wrapped http.ResponseWriter and a pointer to an int containing the status.
func NewResponseWriter(w http.ResponseWriter, t Trace) *httpResponseWriter {
	return &httpResponseWriter{w, t, http.StatusOK, ""}
}

// TraceFromHTTPRequest returns a Trace, given an http.Request. If a distributed trace is described
// in the "X-Trace" header, this context will be continued.
func TraceFromHTTPRequest(layerName string, r *http.Request) Trace {
	log.Printf("got Trace %v r %v\n", layerName, r)
	// start trace, passing in metadata header
	t := NewTraceFromID(layerName, r.Header.Get("X-Trace"), func() KVMap {
		return KVMap{
			"Method":       r.Method,
			"HTTP-Host":    r.Host,
			"URL":          r.URL.Path,
			"Remote-Host":  r.RemoteAddr,
			"Query-String": r.URL.RawQuery,
		}
	})
	// update metadata header for any downstream readers
	log.Printf("Setting X-Trace %v\n", t.MetadataString())
	r.Header.Set("X-Trace", t.MetadataString())
	return t
}
