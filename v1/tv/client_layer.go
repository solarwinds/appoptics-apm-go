// Copyright (C) 2016 Librato, Inc. All rights reserved.
// AppOptics HTTP instrumentation for Go

package tv

import "golang.org/x/net/context"

// BeginQuerySpan returns a Span that reports metadata used by AppOptics to filter
// query latency heatmaps and charts by span name, query statement, DB host and table.
// Parameter "flavor" specifies the flavor of the query statement, such as "mysql", "postgresql", or "mongodb".
// Call or defer the returned Span's End() to time the query's client-side latency.
func BeginQuerySpan(ctx context.Context, spanName, query, flavor, remoteHost string) Span {
	l, _ := BeginSpan(ctx, spanName, "Query", query, "Flavor", flavor, "RemoteHost", remoteHost)
	return l
}

// BeginCacheSpan returns a Span that reports metadata used by AppOptics to filter cache/KV server
// request latency heatmaps and charts by span name, cache operation and hostname.
// Required parameter "op" is meant to report a Redis or Memcached command e.g. "HGET" or "set".
// Filterable hit/miss ratios charts will be available if "hit" is used.
// Optional parameter "key" will display in the trace's details, but will not be indexed.
// Call or defer the returned Span's End() to time the request's client-side latency.
func BeginCacheSpan(ctx context.Context, spanName, op, key, remoteHost string, hit bool) Span {
	l, _ := BeginSpan(ctx, spanName, "KVOp", op, "KVKey", key, "KVHit", hit, "RemoteHost", remoteHost)
	return l
}

// BeginRemoteURLSpan returns a Span that reports metadata used by AppOptics to filter RPC call
// latency heatmaps and charts by span name and URL endpoint. For requests using the "net/http"
// package, BeginHTTPClientSpan also reports this metadata, while also propagating trace context
// metadata headers via http.Request and http.Response.
// Call or defer the returned Span's End() to time the call's client-side latency.
func BeginRemoteURLSpan(ctx context.Context, spanName, remoteURL string) Span {
	l, _ := BeginSpan(ctx, spanName, "IsService", true, "RemoteURL", remoteURL)
	return l
}

// BeginRPCSpan returns a Span that reports metadata used by AppOptics to filter RPC call
// latency heatmaps and charts by span name, protocol, controller, and remote host.
// Call or defer the returned Span's End() to time the call's client-side latency.
func BeginRPCSpan(ctx context.Context, spanName, protocol, controller, remoteHost string) Span {
	l, _ := BeginSpan(ctx, spanName, "IsService", true,
		"RemoteProtocol", protocol, "RemoteHost", remoteHost, "RemoteController", controller)
	return l
}
