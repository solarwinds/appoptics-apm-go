// Copyright (C) 2016 Librato, Inc. All rights reserved.
// TraceView HTTP instrumentation for Go

package tv

import (
	"net/http"

	"golang.org/x/net/context"
)

// HTTPClientLayer is a Layer that aids in reporting HTTP client requests.
//   req, err := http.NewRequest("GET", "http://example.com", nil)
//   l := tv.BeginHTTPClientLayer(ctx, httpReq)
//   defer l.End()
//   // ...
//   resp, err := client.Do(req)
//   l.AddHTTPResponse(resp, err)
//   // ...
type HTTPClientLayer struct{ Layer }

// BeginHTTPClientLayer stores trace metadata in the headers of an HTTP client request, allowing the
// trace to be continued on the other end. It returns a Layer that must have End() called to
// benchmark the client request, and should have AddHTTPResponse(r, err) called to process response
// metadata.
func BeginHTTPClientLayer(ctx context.Context, req *http.Request) HTTPClientLayer {
	if req != nil {
		l := BeginRemoteURLLayer(ctx, "http.Client", req.URL.String())
		req.Header.Set(HTTPHeaderName, l.MetadataString())
		return HTTPClientLayer{Layer: l}
	}
	return HTTPClientLayer{Layer: &nullSpan{}}
}

// AddHTTPResponse adds information from http.Response to this layer. It will also check the HTTP
// response headers and propagate any valid distributed trace context from the end of the HTTP
// server's layer to this one.
func (l HTTPClientLayer) AddHTTPResponse(resp *http.Response, err error) {
	if l.ok() {
		if err != nil {
			l.Err(err)
		}
		if resp != nil {
			l.AddEndArgs("RemoteStatus", resp.StatusCode, "ContentLength", resp.ContentLength)
			if md := resp.Header.Get(HTTPHeaderName); md != "" {
				l.AddEndArgs("Edge", md)
			}
		}
	}
}
