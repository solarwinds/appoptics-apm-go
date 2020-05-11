// Copyright (C) 2016 Librato, Inc. All rights reserved.
// AppOptics HTTP instrumentation for Go

package http

import (
	"net/http"

	"context"

	"github.com/appoptics/appoptics-apm-go/v1/ao/opentelemetry"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/plugin/httptrace"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
)

// ClientSpan is a Span that aids in reporting HTTP client requests.
//   req, err := http.NewRequest("GET", "http://example.com", nil)
//   l := ao.BeginHTTPClientSpan(ctx, httpReq)
//   defer l.End()
//   // ...
//   resp, err := client.Do(req)
//   l.AddHTTPResponse(resp, err)
//   // ...
type ClientSpan struct{ ao.Span }

// Deprecated: use ClientSpan
type HTTPClientSpan = ClientSpan

// BeginHTTPClientSpan stores trace metadata in the headers of an HTTP client request, allowing the
// trace to be continued on the other end. It returns a Span that must have End() called to
// benchmark the client request, and should have AddHTTPResponse(r, err) called to process response
// metadata.
func BeginHTTPClientSpan(ctx context.Context, req *http.Request) ClientSpan {
	if req != nil {
		l := ao.BeginRemoteURLSpan(ctx, "http.Client", req.URL.String())
		req.Header.Set(XTraceHeader, l.MetadataString())

		// Inject OT span context
		otSpan := opentelemetry.Wrapper(l)
		httptrace.Inject(trace.ContextWithSpan(ctx, otSpan), req)

		return ClientSpan{Span: l}
	}
	return ClientSpan{Span: ao.NewNullSpan()}
}

// AddHTTPResponse adds information from http.Response to this span. It will also check the HTTP
// response headers and propagate any valid distributed trace context from the end of the HTTP
// server's span to this one.
func (l ClientSpan) AddHTTPResponse(resp *http.Response, err error) {
	if err != nil {
		l.Err(err)
	}
	if resp != nil {
		l.AddEndArgs(ao.KeyRemoteStatus, resp.StatusCode, ao.KeyContentLength, resp.ContentLength)
		if md := resp.Header.Get(XTraceHeader); md != "" {
			l.AddEndArgs(ao.KeyEdge, md)
		}
	}
}
