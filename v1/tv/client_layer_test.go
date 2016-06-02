// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"testing"
	"time"

	"github.com/appneta/go-appneta/v1/tv"
	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestCacheRPCLayers(t *testing.T) {
	r := traceview.SetTestReporter() // enable test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("myExample"))

	// make a cache request
	l := tv.BeginCacheLayer(ctx, "redis", "INCR", "key31", "redis.net", true)
	// ... client.Incr(key) ...
	time.Sleep(20 * time.Millisecond)
	l.Error("CacheTimeoutError", "Cache request timeout error!")
	l.End()

	// make an RPC request (no trace propagation in this example)
	l = tv.BeginRPCLayer(ctx, "myServiceClient", "thrift", "incrKey", "service.net")
	// ... service.incrKey(key) ...
	time.Sleep(time.Millisecond)
	l.End()

	tv.End(ctx)

	g.AssertGraph(t, r.Bufs, 7, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"myExample", "entry"}: {},
		{"redis", "entry"}: {g.OutEdges{{"myExample", "entry"}}, func(n g.Node) {
			assert.Equal(t, "redis.net", n.Map["RemoteHost"])
			assert.Equal(t, "INCR", n.Map["KVOp"])
			assert.Equal(t, "key31", n.Map["KVKey"])
			assert.Equal(t, true, n.Map["KVHit"])
		}},
		{"redis", "error"}: {g.OutEdges{{"redis", "entry"}}, func(n g.Node) {
			assert.Equal(t, "CacheTimeoutError", n.Map["ErrorClass"])
			assert.Equal(t, "Cache request timeout error!", n.Map["ErrorMsg"])
		}},
		{"redis", "exit"}: {g.OutEdges{{"redis", "error"}}, nil},
		{"myServiceClient", "entry"}: {g.OutEdges{{"myExample", "entry"}}, func(n g.Node) {
			assert.Equal(t, "service.net", n.Map["RemoteHost"])
			assert.Equal(t, "incrKey", n.Map["RemoteController"])
			assert.Equal(t, "thrift", n.Map["RemoteProtocol"])
		}},
		{"myServiceClient", "exit"}: {g.OutEdges{{"myServiceClient", "entry"}}, nil},
		{"myExample", "exit"}:       {g.OutEdges{{"redis", "exit"}, {"myServiceClient", "exit"}, {"myExample", "entry"}}, nil},
	})
}
