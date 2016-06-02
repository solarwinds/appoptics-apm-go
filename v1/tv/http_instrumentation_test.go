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

func handler404(w http.ResponseWriter, r *http.Request)   { w.WriteHeader(404) }
func handler403(w http.ResponseWriter, r *http.Request)   { w.WriteHeader(403) }
func handler200(w http.ResponseWriter, r *http.Request)   {} // do nothing (default should be 200)
func handlerPanic(w http.ResponseWriter, r *http.Request) { panic("panicking!") }

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
	assert.Len(t, response.HeaderMap[tv.HTTPHeaderName], 1)

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {g.OutEdges{}, func(n g.Node) {
			assert.Equal(t, "/hello", n.Map["URL"])
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
			assert.Equal(t, "GET", n.Map["Method"])
			assert.Equal(t, "testq", n.Map["Query-String"])
		}},
		{"http.HandlerFunc", "exit"}: {g.OutEdges{{"http.HandlerFunc", "entry"}}, func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.Equal(t, response.HeaderMap.Get(tv.HTTPHeaderName), n.Map[tv.HTTPHeaderName])
			assert.EqualValues(t, response.Code, n.Map["Status"])
			assert.EqualValues(t, 404, n.Map["Status"])
			assert.Equal(t, "tv_test", n.Map["Controller"])
			assert.Equal(t, "handler404", n.Map["Action"])
		}},
	})
}

func TestHTTPHandler200(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	response := httpTest(handler200)

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {g.OutEdges{}, func(n g.Node) {
			assert.Equal(t, "/hello", n.Map["URL"])
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
			assert.Equal(t, "GET", n.Map["Method"])
			assert.Equal(t, "testq", n.Map["Query-String"])
		}},
		{"http.HandlerFunc", "exit"}: {g.OutEdges{{"http.HandlerFunc", "entry"}}, func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.Len(t, response.HeaderMap[tv.HTTPHeaderName], 1)
			assert.Equal(t, response.HeaderMap[tv.HTTPHeaderName][0], n.Map[tv.HTTPHeaderName])
			assert.EqualValues(t, response.Code, n.Map["Status"])
			assert.EqualValues(t, 200, n.Map["Status"])
			assert.Equal(t, "tv_test", n.Map["Controller"])
			assert.Equal(t, "handler200", n.Map["Action"])
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

// testServer tests creating a layer/trace from inside an HTTP handler (using tv.TraceFromHTTPRequest)
func testServer(t *testing.T, list net.Listener) {
	s := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// create layer from incoming HTTP Request headers, if trace exists
		tr, w := tv.TraceFromHTTPRequestResponse("myHandler", w, req)
		defer tr.End()

		tr.AddEndArgs("NotReported") // odd-length args, should have no effect

		t.Logf("server: got request %v", req)
		l2 := tr.BeginLayer("DBx", "Query", "SELECT *", "RemoteHost", "db.net")
		// Run a query ...
		l2.End()

		w.WriteHeader(403) // return Forbidden
	})}
	assert.NoError(t, s.Serve(list))
}

// same as testServer, but with external tv.HTTPHandler() handler wrapping
func testDoubleWrappedServer(t *testing.T, list net.Listener) {
	s := &http.Server{Handler: http.HandlerFunc(tv.HTTPHandler(func(writer http.ResponseWriter, req *http.Request) {
		// create layer from incoming HTTP Request headers, if trace exists
		tr, w := tv.TraceFromHTTPRequestResponse("myHandler", writer, req)
		defer tr.End()

		t.Logf("server: got request %v", req)
		l2 := tr.BeginLayer("DBx", "Query", "SELECT *", "RemoteHost", "db.net")
		// Run a query ...
		l2.End()

		w.WriteHeader(403) // return Forbidden
	}))}
	assert.NoError(t, s.Serve(list))
}

// testServer200 does not trace and returns a 200.
func testServer200(t *testing.T, list net.Listener) {
	s := &http.Server{Handler: http.HandlerFunc(handler200)}
	assert.NoError(t, s.Serve(list))
}

// testServer403 does not trace and returns a 403.
func testServer403(t *testing.T, list net.Listener) {
	s := &http.Server{Handler: http.HandlerFunc(handler403)}
	assert.NoError(t, s.Serve(list))
}

// simulate panic-catching middleware wrapping tv.HTTPHandler(handlerPanic)
func panicCatchingMiddleware(t *testing.T, f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				t.Logf("panicCatcher caught panic %v", err)
				w.WriteHeader(500)
				w.Write([]byte(fmt.Sprintf("500 Error: %v", err)))
			}
		}()
		f(w, r)
	}
}

// testServerPanic traces a wrapped http.HandlerFunc that panics
func testServerPanic(t *testing.T, list net.Listener) {
	s := &http.Server{Handler: http.HandlerFunc(
		panicCatchingMiddleware(t, tv.HTTPHandler(handlerPanic)))}
	assert.NoError(t, s.Serve(list))
}

// begin an HTTP client span, make an HTTP request, and propagate the trace context manually
func testHTTPClient(t *testing.T, ctx context.Context, method, url string) (*http.Response, error) {
	httpClient := &http.Client{}
	httpReq, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	l, _ := tv.BeginLayer(ctx, "http.Client", "IsService", true, "RemoteURL", url)
	defer l.End()
	httpReq.Header.Set(tv.HTTPHeaderName, l.MetadataString())

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		l.Err(err)
		return resp, err
	}
	defer resp.Body.Close()

	l.AddEndArgs("Edge", resp.Header.Get(tv.HTTPHeaderName))
	return resp, err
}

// create an HTTP client span, make an HTTP request, and propagate the trace using HTTPClientLayer
func testHTTPClientA(t *testing.T, ctx context.Context, method, url string) (*http.Response, error) {
	httpClient := &http.Client{}
	httpReq, err := http.NewRequest(method, url, nil)
	l := tv.BeginHTTPClientLayer(ctx, httpReq)
	defer l.End()
	if err != nil {
		l.Err(err)
		return nil, err
	}

	resp, err := httpClient.Do(httpReq)
	l.AddHTTPResponse(resp, err)
	if err != nil {
		t.Logf("JoinResponse err: %v", err)
		return resp, err
	}
	defer resp.Body.Close()

	return resp, err
}

// create an HTTP client span, make an HTTP request, and propagate the trace using HTTPClientLayer
// and a different exception-handling flow
func testHTTPClientB(t *testing.T, ctx context.Context, method, url string) (*http.Response, error) {
	httpClient := &http.Client{}
	httpReq, err := http.NewRequest(method, url, nil)
	l := tv.BeginHTTPClientLayer(ctx, httpReq)
	if err != nil {
		l.Err(err)
		l.End()
		return nil, err
	}

	resp, err := httpClient.Do(httpReq)
	l.AddHTTPResponse(resp, err)
	l.End()
	if err != nil {
		t.Logf("JoinResponse err: %v", err)
		return resp, err
	}
	defer resp.Body.Close()

	return resp, err
}

type testClientFn func(t *testing.T, ctx context.Context, method, url string) (*http.Response, error)
type testServerFn struct {
	serverFn func(t *testing.T, list net.Listener)
	assertFn func(t *testing.T, bufs [][]byte, resp *http.Response, url, method string, port, status int)
	status   int
}

var testHTTPSvr = testServerFn{testServer, assertHTTPRequestGraph, 403}
var testHTTPSvr200 = testServerFn{testServer200, assertHTTPRequestUntracedGraph, 200}
var testHTTPSvr403 = testServerFn{testServer403, assertHTTPRequestUntracedGraph, 403}
var testHTTPSvrPanic = testServerFn{testServerPanic, assertHTTPRequestPanic, 200}

var badURL = "%gh&%ij" // url.Parse() will return error
var invalidPortURL = "http://0.0.0.0:888888"

func TestTraceHTTP(t *testing.T)              { testHTTP(t, "GET", false, testHTTPClient, testHTTPSvr) }
func TestTraceHTTPHelperA(t *testing.T)       { testHTTP(t, "GET", false, testHTTPClientA, testHTTPSvr) }
func TestTraceHTTPHelperB(t *testing.T)       { testHTTP(t, "GET", false, testHTTPClientB, testHTTPSvr) }
func TestTraceHTTP200(t *testing.T)           { testHTTP(t, "GET", false, testHTTPClient, testHTTPSvr200) }
func TestTraceHTTPHelperA200(t *testing.T)    { testHTTP(t, "GET", false, testHTTPClientA, testHTTPSvr200) }
func TestTraceHTTPHelperB200(t *testing.T)    { testHTTP(t, "GET", false, testHTTPClientB, testHTTPSvr200) }
func TestTraceHTTP403(t *testing.T)           { testHTTP(t, "GET", false, testHTTPClient, testHTTPSvr403) }
func TestTraceHTTPHelperA403(t *testing.T)    { testHTTP(t, "GET", false, testHTTPClientA, testHTTPSvr403) }
func TestTraceHTTPHelperB403(t *testing.T)    { testHTTP(t, "GET", false, testHTTPClientB, testHTTPSvr403) }
func TestTraceHTTPPost(t *testing.T)          { testHTTP(t, "POST", false, testHTTPClient, testHTTPSvr) }
func TestTraceHTTPHelperPostA(t *testing.T)   { testHTTP(t, "POST", false, testHTTPClientA, testHTTPSvr) }
func TestTraceHTTPHelperPostB(t *testing.T)   { testHTTP(t, "POST", false, testHTTPClientB, testHTTPSvr) }
func TestTraceHTTPBadRequest(t *testing.T)    { testHTTP(t, "GET", true, testHTTPClient, testHTTPSvr) }
func TestTraceHTTPHelperBadReqA(t *testing.T) { testHTTP(t, "GET", true, testHTTPClientA, testHTTPSvr) }
func TestTraceHTTPHelperBadReqB(t *testing.T) { testHTTP(t, "GET", true, testHTTPClientB, testHTTPSvr) }
func TestTraceHTTPPanic(t *testing.T)         { testHTTP(t, "GET", false, testHTTPClient, testHTTPSvrPanic) }
func TestTraceHTTPPanicA(t *testing.T)        { testHTTP(t, "GET", false, testHTTPClientA, testHTTPSvrPanic) }
func TestTraceHTTPPanicB(t *testing.T)        { testHTTP(t, "GET", false, testHTTPClientB, testHTTPSvrPanic) }

// launch a test HTTP server and trace an HTTP request to it
func testHTTP(t *testing.T, method string, badReq bool, clientFn testClientFn, server testServerFn) {
	ln, err := net.Listen("tcp", ":0") // pick an unallocated port
	assert.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	go server.serverFn(t, ln) // start test server

	r := traceview.SetTestReporter() // set up test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("httpTest"))
	// make request to URL of test server
	url := fmt.Sprintf("http://127.0.0.1:%d/test?qs=1", port)
	if badReq {
		url = badURL // causes url.Parse() in http.NewRequest() to fail
	}
	resp, err := clientFn(t, ctx, method, url)
	tv.EndTrace(ctx)

	if badReq { // handle case where http.NewRequest() returned nil
		assert.Error(t, err)
		assert.Nil(t, resp)
		g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
			{"httpTest", "entry"}: {},
			{"httpTest", "exit"}:  {g.OutEdges{{"httpTest", "entry"}}, nil},
		})
		return
	}
	// handle case where http.Client.Do() did not return an error
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	server.assertFn(t, r.Bufs, resp, url, method, port, server.status)
}

// assert traces that hit testServer, which uses the HTTP server instrumentation.
func assertHTTPRequestGraph(t *testing.T, bufs [][]byte, resp *http.Response, url, method string, port, status int) {
	assert.Len(t, resp.Header[tv.HTTPHeaderName], 1)
	assert.Equal(t, status, resp.StatusCode)

	g.AssertGraph(t, bufs, 8, map[g.MatchNode]g.AssertNode{
		{"httpTest", "entry"}: {},
		{"http.Client", "entry"}: {g.OutEdges{{"httpTest", "entry"}}, func(n g.Node) {
			assert.Equal(t, true, n.Map["IsService"])
			assert.Equal(t, url, n.Map["RemoteURL"])
		}},
		{"http.Client", "exit"}: {g.OutEdges{{"myHandler", "exit"}, {"http.Client", "entry"}}, nil},
		{"myHandler", "entry"}: {g.OutEdges{{"http.Client", "entry"}}, func(n g.Node) {
			assert.Equal(t, "/test", n.Map["URL"])
			assert.Equal(t, fmt.Sprintf("127.0.0.1:%d", port), n.Map["HTTP-Host"])
			assert.Equal(t, "qs=1", n.Map["Query-String"])
			assert.Equal(t, method, n.Map["Method"])
		}},
		{"myHandler", "exit"}: {g.OutEdges{{"DBx", "exit"}, {"myHandler", "entry"}}, func(n g.Node) {
			assert.Equal(t, status, n.Map["Status"])
		}},
		{"DBx", "entry"}: {g.OutEdges{{"myHandler", "entry"}}, func(n g.Node) {
			assert.Equal(t, "SELECT *", n.Map["Query"])
			assert.Equal(t, "db.net", n.Map["RemoteHost"])
		}},
		{"DBx", "exit"}:      {g.OutEdges{{"DBx", "entry"}}, nil},
		{"httpTest", "exit"}: {g.OutEdges{{"http.Client", "exit"}, {"httpTest", "entry"}}, nil},
	})
}

// assert traces of an HTTP client to untraced servers testServer200 and testServer403.
func assertHTTPRequestUntracedGraph(t *testing.T, bufs [][]byte, resp *http.Response, url, method string, port, status int) {
	assert.NotContains(t, resp.Header[tv.HTTPHeaderName], "Header")
	assert.Equal(t, status, resp.StatusCode)

	g.AssertGraph(t, bufs, 4, map[g.MatchNode]g.AssertNode{
		{"httpTest", "entry"}: {},
		{"http.Client", "entry"}: {g.OutEdges{{"httpTest", "entry"}}, func(n g.Node) {
			assert.Equal(t, true, n.Map["IsService"])
			assert.Equal(t, url, n.Map["RemoteURL"])
		}},
		{"http.Client", "exit"}: {g.OutEdges{{"http.Client", "entry"}}, nil},
		{"httpTest", "exit"}:    {g.OutEdges{{"http.Client", "exit"}, {"httpTest", "entry"}}, nil},
	})
}

// assert traces that hit a TV-wrapped, panicking http Handler.
func assertHTTPRequestPanic(t *testing.T, bufs [][]byte, resp *http.Response, url, method string, port, status int) {

	g.AssertGraph(t, bufs, 7, map[g.MatchNode]g.AssertNode{
		{"httpTest", "entry"}: {},
		{"http.Client", "entry"}: {g.OutEdges{{"httpTest", "entry"}}, func(n g.Node) {
			assert.Equal(t, true, n.Map["IsService"])
			assert.Equal(t, url, n.Map["RemoteURL"])
		}},
		{"http.Client", "exit"}: {g.OutEdges{{"http.HandlerFunc", "exit"}, {"http.Client", "entry"}}, nil},
		{"http.HandlerFunc", "entry"}: {g.OutEdges{{"http.Client", "entry"}}, func(n g.Node) {
			assert.Equal(t, "/test", n.Map["URL"])
			assert.Equal(t, fmt.Sprintf("127.0.0.1:%d", port), n.Map["HTTP-Host"])
			assert.Equal(t, "qs=1", n.Map["Query-String"])
			assert.Equal(t, method, n.Map["Method"])
		}},
		{"http.HandlerFunc", "error"}: {g.OutEdges{{"http.HandlerFunc", "entry"}}, func(n g.Node) {
			assert.Equal(t, "panic", n.Map["ErrorClass"])
			assert.Equal(t, "panicking!", n.Map["ErrorMsg"])
		}},
		{"http.HandlerFunc", "exit"}: {g.OutEdges{{"http.HandlerFunc", "error"}}, func(n g.Node) {
			assert.Equal(t, "tv_test", n.Map["Controller"])
			assert.Equal(t, "handlerPanic", n.Map["Action"])
			assert.Equal(t, status, n.Map["Status"])
		}},
		{"httpTest", "exit"}: {g.OutEdges{{"http.Client", "exit"}, {"httpTest", "entry"}}, nil},
	})
}

func TestTraceHTTPError(t *testing.T)            { testTraceHTTPError(t, "GET", false, testHTTPClient) }
func TestTraceHTTPErrorA(t *testing.T)           { testTraceHTTPError(t, "GET", false, testHTTPClientA) }
func TestTraceHTTPErrorB(t *testing.T)           { testTraceHTTPError(t, "GET", false, testHTTPClientB) }
func TestTraceHTTPErrorBadRequest(t *testing.T)  { testTraceHTTPError(t, "GET", true, testHTTPClient) }
func TestTraceHTTPErrorABadRequest(t *testing.T) { testTraceHTTPError(t, "GET", true, testHTTPClientA) }
func TestTraceHTTPErrorBBadRequest(t *testing.T) { testTraceHTTPError(t, "GET", true, testHTTPClientB) }

// test making an HTTP request that causes http.Client.Do() to fail
func testTraceHTTPError(t *testing.T, method string, badReq bool, clientFn testClientFn) {
	r := traceview.SetTestReporter() // set up test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("httpTest"))
	url := invalidPortURL // make HTTP req to invalid port
	if badReq {
		url = badURL // causes url.Parse() in http.NewRequest() to fail
	}
	resp, err := clientFn(t, ctx, method, url)
	tv.EndTrace(ctx)

	assert.Error(t, err)
	assert.Nil(t, resp)

	if badReq { // handle case where http.NewRequest() returned nil
		g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
			{"httpTest", "entry"}: {},
			{"httpTest", "exit"}:  {g.OutEdges{{"httpTest", "entry"}}, nil},
		})
		return
	}
	// handle case where http.Client.Do() returned an error
	g.AssertGraph(t, r.Bufs, 5, map[g.MatchNode]g.AssertNode{
		{"httpTest", "entry"}: {},
		{"http.Client", "entry"}: {g.OutEdges{{"httpTest", "entry"}}, func(n g.Node) {
			assert.Equal(t, true, n.Map["IsService"])
			assert.Equal(t, url, n.Map["RemoteURL"])
		}},
		{"http.Client", "error"}: {g.OutEdges{{"http.Client", "entry"}}, func(n g.Node) {
			assert.Equal(t, "error", n.Map["ErrorClass"])
			assert.Contains(t, n.Map["ErrorMsg"], "dial tcp: invalid port 888888")
		}},
		{"http.Client", "exit"}: {g.OutEdges{{"http.Client", "error"}}, nil},
		{"httpTest", "exit"}:    {g.OutEdges{{"http.Client", "exit"}, {"httpTest", "entry"}}, nil},
	})
}

func TestDoubleWrappedHTTPRequest(t *testing.T) {
	list, err := net.Listen("tcp", ":0") // pick an unallocated port
	assert.NoError(t, err)
	port := list.Addr().(*net.TCPAddr).Port
	go testDoubleWrappedServer(t, list) // start test server

	r := traceview.SetTestReporter() // set up test reporter
	ctx := tv.NewContext(context.Background(), tv.NewTrace("httpTest"))
	url := fmt.Sprintf("http://127.0.0.1:%d/test?qs=1", port)
	resp, err := testHTTPClient(t, ctx, "GET", url)
	t.Logf("response: %v", resp)
	tv.EndTrace(ctx)

	assert.NoError(t, err)
	assert.Len(t, resp.Header[tv.HTTPHeaderName], 1)
	assert.Equal(t, 403, resp.StatusCode)

	g.AssertGraph(t, r.Bufs, 10, map[g.MatchNode]g.AssertNode{
		{"httpTest", "entry"}: {},
		{"http.Client", "entry"}: {g.OutEdges{{"httpTest", "entry"}}, func(n g.Node) {
			assert.Equal(t, true, n.Map["IsService"])
			assert.Equal(t, url, n.Map["RemoteURL"])
		}},
		{"http.Client", "exit"}: {g.OutEdges{{"http.HandlerFunc", "exit"}, {"http.Client", "entry"}}, nil},
		{"http.HandlerFunc", "entry"}: {g.OutEdges{{"http.Client", "entry"}}, func(n g.Node) {
			assert.Equal(t, "/test", n.Map["URL"])
			assert.Equal(t, fmt.Sprintf("127.0.0.1:%d", port), n.Map["HTTP-Host"])
			assert.Equal(t, "qs=1", n.Map["Query-String"])
			assert.Equal(t, "GET", n.Map["Method"])
		}},
		{"http.HandlerFunc", "exit"}: {g.OutEdges{{"myHandler", "exit"}, {"http.HandlerFunc", "entry"}}, func(n g.Node) {
			assert.Equal(t, 403, n.Map["Status"])
			assert.Equal(t, "tv_test", n.Map["Controller"])
			assert.Equal(t, "testDoubleWrappedServer.func1", n.Map["Action"])
		}},
		{"myHandler", "entry"}: {g.OutEdges{{"http.HandlerFunc", "entry"}}, func(n g.Node) {
			assert.Equal(t, "/test", n.Map["URL"])
			assert.Equal(t, fmt.Sprintf("127.0.0.1:%d", port), n.Map["HTTP-Host"])
			assert.Equal(t, "qs=1", n.Map["Query-String"])
			assert.Equal(t, "GET", n.Map["Method"])
		}},
		{"myHandler", "exit"}: {g.OutEdges{{"DBx", "exit"}, {"myHandler", "entry"}}, func(n g.Node) {
			assert.Equal(t, 403, n.Map["Status"])
		}},
		{"DBx", "entry"}: {g.OutEdges{{"myHandler", "entry"}}, func(n g.Node) {
			assert.Equal(t, "SELECT *", n.Map["Query"])
			assert.Equal(t, "db.net", n.Map["RemoteHost"])
		}},
		{"DBx", "exit"}:      {g.OutEdges{{"DBx", "entry"}}, nil},
		{"httpTest", "exit"}: {g.OutEdges{{"http.Client", "exit"}, {"httpTest", "entry"}}, nil},
	})
}
