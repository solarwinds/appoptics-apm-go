// Copyright (C) 2016 AppNeta, Inc. All rights reserved.
// TraceView HTTP instrumentation for Go

package tv

import (
	"net/http"

	"golang.org/x/net/context"
)

type HTTPClientLayer struct {
	Layer
	// XXX prevent child spans?
}

// BeginHTTPClientLayer stores trace metadata in the headers of an HTTP
// client request, allowing the trace to be continued on the other end.
func BeginHTTPClientLayer(ctx context.Context, r *http.Request) HTTPClientLayer {
	if r != nil {
		l, _ := BeginLayer(ctx, "http.Client", "IsService", true, "RemoteURL", r.URL.String())
		r.Header.Set("X-Trace", l.MetadataString())
		return HTTPClientLayer{Layer: l}
	}
	return HTTPClientLayer{Layer: &nullSpan{}}
}

// JoinHTTPResponse propagates the distributing tracing context
// from the end of a remote HTTP layer to this layer.
func (l HTTPClientLayer) JoinHTTPResponse(r *http.Response, err error) {
	if err != nil {
		l.Err(err)
	}
	if r != nil {
		// TODO also test when no X-Trace header in response, or req fails
		if md := r.Header.Get("X-Trace"); md != "" {
			l.AddEndArgs("Edge", md)
		}
	}
}
