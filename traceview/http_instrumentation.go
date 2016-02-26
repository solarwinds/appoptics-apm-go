// TraceView HTTP instrumentation for Go

package traceview

import (
	"net/http"
)

var layer = "go_http"

// Wraps an http handler function with entry / exit events.
// Returns a new function that can be used in its place.
func InstrumentedHttpHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var ctx *Context
		var event *Event
		xtrace_header := r.Header.Get("X-Trace")
		sampled, sample_rate, sample_source := ShouldTraceRequest(layer, xtrace_header)
		status := http.StatusOK

		if sampled {
			// Create context:
			// For more complete instrumentation, this context would be stored somewhere.
			if len(xtrace_header) > 0 {
				// Continuing trace:
				ctx = NewContextFromMetaDataString(xtrace_header)
			} else {
				// New trace:
				ctx = NewContext()
			}

			// Create and report entry event:
			event = entryEvent(ctx, r, sample_rate, sample_source)
			if len(xtrace_header) > 0 {
				event.AddEdgeFromMetaDataString(xtrace_header)
			}
			event.Report(ctx)

			// Create exit event, but do not report yet. Just add its X-Trace header:
			event = exitEvent(ctx)
			w.Header().Set("X-Trace", event.MetaDataString())

			w = httpResponseWriter{w, &status}
		}

		// Call original HTTP handler:
		handler(w, r)

		if sampled {
			// Add status code and report exit event:
			event.AddInt("Status", status)
			event.Report(ctx)
		}
	}
}

// Returns an HTTP entry event:
func entryEvent(ctx *Context, r *http.Request, sample_rate, sample_source int) *Event {
	e := ctx.NewEvent(LabelEntry, layer)
	e.AddString("Method", r.Method)
	e.AddString("HTTP-Host", r.Host)
	e.AddString("URL", r.URL.Path)
	e.AddString("Remote-Host", r.RemoteAddr)

	if len(r.URL.RawQuery) > 0 {
		e.AddString("Query-String", r.URL.RawQuery)
	}

	e.AddInt("SampleRate", sample_rate)
	e.AddInt("SampleSource", sample_source)

	return e
}

// Returns an HTTP exit event:
func exitEvent(ctx *Context) *Event {
	e := ctx.NewEvent(LabelExit, layer)
	e.AddEdge(ctx)
	return e
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
