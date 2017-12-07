// Copyright (C) 2016 Librato, Inc. All rights reserved.

package tv_test

import (
	"testing"
	"time"

	"github.com/librato/go-traceview/v1/tv"
	g "github.com/librato/go-traceview/v1/tv/internal/graphtest"
	"github.com/librato/go-traceview/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestCacheRPCSpans(t *testing.T) {
	r := traceview.SetTestReporter() // enable test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("myExample"))

	// make a cache request
	l := tv.BeginCacheSpan(ctx, "redis", "INCR", "key31", "redis.net", true)
	// ... client.Incr(key) ...
	time.Sleep(20 * time.Millisecond)
	l.Error("CacheTimeoutError", "Cache request timeout error!")
	l.End()

	// make an RPC request (no trace propagation in this example)
	l = tv.BeginRPCSpan(ctx, "myServiceClient", "thrift", "incrKey", "service.net")
	// ... service.incrKey(key) ...
	time.Sleep(time.Millisecond)
	l.End()

	tv.End(ctx)

	r.Close(7)
	g.AssertGraph(t, r.Bufs, 7, g.AssertNodeMap{
		// entry event should have no edges
		{"myExample", "entry"}: {},
		{"redis", "entry"}: {Edges: g.Edges{{"myExample", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, "redis.net", n.Map["RemoteHost"])
			assert.Equal(t, "INCR", n.Map["KVOp"])
			assert.Equal(t, "key31", n.Map["KVKey"])
			assert.Equal(t, true, n.Map["KVHit"])
		}},
		{"redis", "error"}: {Edges: g.Edges{{"redis", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, "CacheTimeoutError", n.Map["ErrorClass"])
			assert.Equal(t, "Cache request timeout error!", n.Map["ErrorMsg"])
		}},
		{"redis", "exit"}: {Edges: g.Edges{{"redis", "error"}}},
		{"myServiceClient", "entry"}: {Edges: g.Edges{{"myExample", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, "service.net", n.Map["RemoteHost"])
			assert.Equal(t, "incrKey", n.Map["RemoteController"])
			assert.Equal(t, "thrift", n.Map["RemoteProtocol"])
		}},
		{"myServiceClient", "exit"}: {Edges: g.Edges{{"myServiceClient", "entry"}}},
		{"myExample", "exit"}:       {Edges: g.Edges{{"redis", "exit"}, {"myServiceClient", "exit"}, {"myExample", "entry"}}},
	})
}
