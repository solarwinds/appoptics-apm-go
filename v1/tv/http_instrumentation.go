// Copyright (C) 2016 AppNeta, Inc. All rights reserved.
// TraceView HTTP instrumentation for Go

package tv

import "net/http"

var httpLayerName = "net/http"

// HTTPHandler wraps an http handler function with entry / exit events,
// returning a new function that can be used in its place.
func HTTPHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		t := NewTraceFromID(httpLayerName, r.Header.Get("X-Trace"), func() KVMap {
			return KVMap{
				"Method":       r.Method,
				"HTTP-Host":    r.Host,
				"URL":          r.URL.Path,
				"Remote-Host":  r.RemoteAddr,
				"Query-String": r.URL.RawQuery,
			}
		})
		// add exit event's X-Trace header:
		if t.IsTracing() {
			md := t.ExitMetadata()
			w.Header().Set("X-Trace", md)
		}

		// wrap writer with status-observing writer
		status := http.StatusOK
		w = httpResponseWriter{w, &status}

		// Add status code and report exit event
		defer t.EndCallback(func() KVMap { return KVMap{"Status": status} })

		// Call original HTTP handler:
		handler(w, r)
	}
}

// httpResponseWriter observes calls to another http.ResponseWriter that change
// the HTTP status code.
type httpResponseWriter struct {
	http.ResponseWriter
	status *int
}

func (w httpResponseWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	*w.status = status
}
