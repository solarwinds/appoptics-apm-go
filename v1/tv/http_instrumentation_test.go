// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/appneta/go-appneta/v1/tv"
	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func handler404(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }
func handler200(w http.ResponseWriter, r *http.Request) {} // do nothing (default should be 200)

func httpTest(f http.HandlerFunc) *httptest.ResponseRecorder {
	h := http.HandlerFunc(tv.HTTPHandler(f))
	// test a single GET request
	req, _ := http.NewRequest("GET", "http://test.com/hello?testq", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHTTPHandler404(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	response := httpTest(handler404)
	assert.Len(t, response.HeaderMap["X-Trace"], 1)

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"net/http", "entry"}: {g.OutEdges{}, func(n g.Node) {
			assert.Equal(t, "/hello", n.Map["URL"])
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
			assert.Equal(t, "GET", n.Map["Method"])
			assert.Equal(t, "testq", n.Map["Query-String"])
		}},
		{"net/http", "exit"}: {g.OutEdges{{"net/http", "entry"}}, func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
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
		{"net/http", "entry"}: {g.OutEdges{}, func(n g.Node) {
			assert.Equal(t, "/hello", n.Map["URL"])
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
			assert.Equal(t, "GET", n.Map["Method"])
			assert.Equal(t, "testq", n.Map["Query-String"])
		}},
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

func testServer(t *testing.T, list net.Listener) {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) { // tv.HTTPHandler(
		// create layer from incoming HTTP Request headers, if trace exists
		tr := tv.TraceFromHTTPRequest(req)
		w, status := tv.NewResponseWriter(w)
		defer tr.End("Status", status)
		w.Header().Set("X-Trace", tr.ExitMetadata()) // set exit header

		t.Logf("server: got request %v", req)
		l2 := tr.BeginLayer("DBx", "Query", "SELECT *", "RemoteHost", "db.net")
		// Run a query ...
		l2.End()

		w.WriteHeader(403) // return Forbidden
	})
	assert.NoError(t, http.Serve(list, nil))
}

// create an HTTP client span, make an HTTP request, and propagate the trace context
func testClient(ctx context.Context, url string) (*http.Response, error) {
	l, _ := tv.BeginLayer(ctx, "http.Client", "IsService", true, "RemoteURL", url)

	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("GET", url, nil)
	httpReq.Header["X-Trace"] = []string{l.MetadataString()}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		l.Err(err)
	}
	// TODO also test when no X-Trace header in response
	l.End("Edge", resp.Header["X-Trace"])
	return resp, err
}

func TestHTTPRequest(t *testing.T) {
	list, err := net.Listen("tcp", ":0") // pick an unallocated port
	assert.NoError(t, err)
	port := list.Addr().(*net.TCPAddr).Port
	go testServer(t, list) // start test server

	r := traceview.SetTestReporter() // set up test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("httpTest"))
	url := fmt.Sprintf("http://127.0.0.1:%d/test?qs=1", port)
	resp, err := testClient(ctx, url)
	tv.EndTrace(ctx)

	assert.NoError(t, err)
	assert.Len(t, resp.Header["X-Trace"], 1)
	assert.Equal(t, 403, resp.StatusCode)

	g.AssertGraph(t, r.Bufs, 8, map[g.MatchNode]g.AssertNode{
		{"httpTest", "entry"}: {},
		{"http.Client", "entry"}: {g.OutEdges{{"httpTest", "entry"}}, func(n g.Node) {
			assert.Equal(t, true, n.Map["IsService"])
			assert.Equal(t, url, n.Map["RemoteURL"])
		}},
		{"http.Client", "exit"}: {g.OutEdges{{"net/http", "exit"}, {"http.Client", "entry"}}, nil},
		{"net/http", "entry"}: {g.OutEdges{{"http.Client", "entry"}}, func(n g.Node) {
			assert.Equal(t, "/test", n.Map["URL"])
			assert.Equal(t, fmt.Sprintf("127.0.0.1:%d", port), n.Map["HTTP-Host"])
			assert.Equal(t, "qs=1", n.Map["Query-String"])
			assert.Equal(t, "GET", n.Map["Method"])
		}},
		{"net/http", "exit"}: {g.OutEdges{{"DBx", "exit"}, {"net/http", "entry"}}, func(n g.Node) {
			assert.Equal(t, 403, n.Map["Status"])
		}},
		{"DBx", "entry"}: {g.OutEdges{{"net/http", "entry"}}, func(n g.Node) {
			assert.Equal(t, "SELECT *", n.Map["Query"])
			assert.Equal(t, "db.net", n.Map["RemoteHost"])
		}},
		{"DBx", "exit"}:      {g.OutEdges{{"DBx", "entry"}}, nil},
		{"httpTest", "exit"}: {g.OutEdges{{"http.Client", "exit"}, {"httpTest", "entry"}}, nil},
	})
}
