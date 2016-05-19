// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/appneta/go-appneta/v1/tv"
	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
)

func handler404(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }
func handler200(w http.ResponseWriter, r *http.Request) {} // do nothing (default should be 200)

func httpTest(f http.HandlerFunc) *httptest.ResponseRecorder {
	h := http.HandlerFunc(tv.HTTPHandler(f))
	// test a single GET request
	req, _ := http.NewRequest("GET", "", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHTTPHandler404(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	response := httpTest(handler404)

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"net/http", "entry"}: {},
		{"net/http", "exit"}: {g.OutEdges{{"net/http", "entry"}}, func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.Len(t, response.HeaderMap["X-Trace"], 1)
			assert.Equal(t, response.HeaderMap["X-Trace"][0], n.Map["X-Trace"])
			assert.EqualValues(t, response.Code, n.Map["Status"])
			assert.EqualValues(t, 404, n.Map["Status"])
		}},
	})
}

func TestHTTPHandler200(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	response := httpTest(handler200)

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"net/http", "entry"}: {},
		{"net/http", "exit"}: {g.OutEdges{{"net/http", "entry"}}, func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.Len(t, response.HeaderMap["X-Trace"], 1)
			assert.Equal(t, response.HeaderMap["X-Trace"][0], n.Map["X-Trace"])
			assert.EqualValues(t, response.Code, n.Map["Status"])
			assert.EqualValues(t, 200, n.Map["Status"])
		}},
	})
}

func TestHTTPHandlerNoTrace(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	r.ShouldTrace = false
	httpTest(handler404)

	// tracing disabled, shouldn't report anything
	assert.Len(t, r.Bufs, 0)
}
