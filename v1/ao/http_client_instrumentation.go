// Copyright (C) 2016 Librato, Inc. All rights reserved.
// AppOptics HTTP instrumentation for Go

package ao

import (
	"net/http"

	"context"
	// "github.com/appoptics/appoptics-apm-go/v1/ao/opentelemetry"
	// "go.opentelemetry.io/otel/api/trace"
	// "go.opentelemetry.io/otel/plugin/httptrace"
)

// HTTPClientSpan is a Span that aids in reporting HTTP client requests.
//   req, err := http.NewRequest("GET", "http://example.com", nil)
//   l := ao.BeginHTTPClientSpan(ctx, httpReq)
//   defer l.End()
//   // ...
//   resp, err := client.Do(req)
//   l.AddHTTPResponse(resp, err)
//   // ...
type HTTPClientSpan struct{ Span }

// BeginHTTPClientSpan stores trace metadata in the headers of an HTTP client request, allowing the
// trace to be continued on the other end. It returns a Span that must have End() called to
// benchmark the client request, and should have AddHTTPResponse(r, err) called to process response
// metadata.
func BeginHTTPClientSpan(ctx context.Context, req *http.Request) HTTPClientSpan {
	if req != nil {
		l := BeginRemoteURLSpan(ctx, "http.Client", req.URL.String())
		req.Header.Set(HTTPHeaderName, l.MetadataString())

		// // Inject OT span context TODO
		// otSpan := opentelemetry.Wrapper(l)
		// httptrace.Inject(trace.ContextWithSpan(ctx, otSpan), req)

		return HTTPClientSpan{Span: l}
	}
	return HTTPClientSpan{Span: nullSpan{}}
}

// AddHTTPResponse adds information from http.Response to this span. It will also check the HTTP
// response headers and propagate any valid distributed trace context from the end of the HTTP
// server's span to this one.
func (l HTTPClientSpan) AddHTTPResponse(resp *http.Response, err error) {
	if l.ok() {
		if err != nil {
			l.Err(err)
		}
		if resp != nil {
			l.AddEndArgs(keyRemoteStatus, resp.StatusCode, keyContentLength, resp.ContentLength)
			if md := resp.Header.Get(HTTPHeaderName); md != "" {
				l.AddEndArgs(keyEdge, md)
			}
		}
	}
}
