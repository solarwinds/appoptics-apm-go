// Package tv implements a simple API for distributed tracing using AppNeta's TraceView.
//
// Usage
//
// To trace a basic HTTP web service, you may use our HttpHandler wrapper to profile the
// performance of your web server. This will automatically propagate a TraceView distributed
// trace context from the request headers through to the response headers, if one exists.
//
//  package main
//
//  import (
//      "github.com/appneta/go-traceview/v1/tv"
//      "math/rand"
//      "net/http"
//      "time"
//  )
//
//  func slow_handler(w http.ResponseWriter, r *http.Request) {
//      time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)
//  }
//
//  func main() {
//      http.HandleFunc("/", tv.HttpHandler(slow_handler))
//      http.ListenAndServe(":8899", nil)
//  }
//
// For more advanced use cases, you may need to begin or continue traces yourself. To do so
// first start a trace, and then create a series of Layer or Profile spans to capture the
// timings of different parts of your app.
//
// In TraceView, there are two types of measurable spans: a "Layer" can measure an entire
// controller method invocation, a single DB query or cache request, or an outgoing HTTP or RPC
// request. A "Profile" typically measures a single function call or nested block. Both Layer and
// Profile spans can be created as children of other Layer spans, but Profiles cannot have child
// spans. The difference between them is that a "Profile" is meant as a way of accounting for time
// spent inside of a Layer, but does not subtract from its parent's total measured Layer time when
// browsing performance data in TraceView. Instead, a breakdown of Profiles associated with a
// given Layer appears in TraceView's dashboard for that layer in your app.
//
//  // create trace and bind to new context
//  ctx := tv.NewContext(context.Background(), tv.NewTrace("myApp"))
//  // create new layer span for this trace
//  l := tv.BeginLayer(ctx, "myLayer")
//
//  // profile a function call, or part of a Layer
//  slowFunc := function() {
//      defer l.BeginProfile("slowFunc").End()
//      // ... do something slow
//  }
//  slowFunc()
//
//  // Start a new span, given a parent layer
//  q1L := l.BeginLayer("myDB", "Query", "SELECT * FROM tbl1")
//  // perform a query
//  q1L.End()
//
//  // Start a new span, given a context.Context
//  q2L, _ := tv.BeginLayer(ctx, "myDB", "Query", "SELECT * FROM tbl2")
//  // perform a query
//  q2L.End()
//
//  l.End()
//  tv.EndTrace(ctx)
//
// Distributed tracing requires a context
//
// A trace is defined by a context (associated with a globally unique trace ID) that is persisted
// across the different hosts, processes, and methods that make up your app's stack when serving
// a request. Thus to start a new span you need either a Trace or another Layer to begin from.
//
// Following Google's advice, at AppNeta we also pass a Context parameter as the first argument to
// functions on the call path between incoming and outgoing requests (read more at
// https://blog.golang.org/context). We provide a set of methods that allow a Trace to be
// associated with the context.Context interface from golang.org/x/net/context for interoperability
// with other compatible Context implementations and helpers like Gizmo's server.ContextHandler.
//
//  // create trace and bind to new context
//  ctx := tv.NewContext(context.Background(), tv.NewTrace("myApp"))
//
//  // instrument a DB query
//  l := tv.BeginLayer(ctx, "DBx", "Query", "SELECT * FROM tbl")
//  // .. execute query ..
//  l.End()
//
//  // end trace
//  tv.EndTrace(ctx)
//
// Configuration
//
// The GO_TRACEVIEW_TRACING_MODE environment variable may be set to "always", "through", or "never".
// Mode "always" is the default, and will instruct TraceView to consider sampling every inbound request for tracing.
// Mode "through" will only continue traces started upstream by inbound requests, when a trace ID is available
// in the request metadata (e.g. an "X-Trace" HTTP header).
// Mode "never" will disable tracing, and will neither start nor continue traces.
//
// Building and installing
//
// By default, no tracing occurs unless you build your app with the build tag "traceview". Without the build tag,
// calls to this API become no-ops, allowing for testing and development and precise control over which deployments
// are traced. To use TraceView with this library, you need to build and run your app on a host with TraceView installed.
//
// To do so, sign up for a TraceView account at https://www.appneta.com/products/traceview/ and follow the
// installation instructions, which will set up the AppNeta's "tracelyzer" host agent and the "liboboe" library dependency
// on your platform. (These dependencies are available as RPM and DEB packages, and many of our customers use Chef
// or similar systems to configure and install them.) Once the liboboe-dev (or liboboe-devel) package is installed you can
// build your service with tracing enabled, for example:
//
//  $ go build -tags traceview
//
// Support
//
// While we use TraceView to trace our own production Go services, this version of our Go instrumentation is currently in beta
// and under active development. We welcome your feedback, issues and feature requests, and please contact us at go@appneta.com!
package tv
