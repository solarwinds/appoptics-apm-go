// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/appneta/go-traceview/v1/tv"
	g "github.com/appneta/go-traceview/v1/tv/internal/graphtest"
	"github.com/appneta/go-traceview/v1/tv/internal/traceview"
)

func httpTest() *httptest.ResponseRecorder {
	// create & wrap 404 handler
	f := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}
	h := http.HandlerFunc(tv.HttpHandler(f))
	//sm := http.NewServeMux()
	//sm.HandleFunc("/", h)

	// test a single GET request
	req, _ := http.NewRequest("GET", "", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHttpHandler(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	response := httpTest()

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"net/http", "entry"}: {},
		{"net/http", "exit"}: {g.OutEdges{{"net/http", "entry"}}, func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.Len(t, response.HeaderMap["X-Trace"], 1)
			assert.Equal(t, response.HeaderMap["X-Trace"][0], n.Map["X-Trace"])
		}},
	})
}

func TestHttpHandlerNoTrace(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	r.ShouldTrace = false
	httpTest()

	// tracing disabled, shouldn't report anything
	assert.Len(t, r.Bufs, 0)
}
