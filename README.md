
# TraceView for Go

[![GoDoc](https://godoc.org/github.com/appneta/go-traceview/v1/tv?status.svg)](https://godoc.org/github.com/appneta/go-traceview/v1/tv)
[![Build Status](https://travis-ci.org/appneta/go-traceview.svg?branch=master)](https://travis-ci.org/appneta/go-traceview)
[![Coverage Status](https://coveralls.io/repos/github/appneta/go-traceview/badge.svg?branch=master)](https://coveralls.io/github/appneta/go-traceview?branch=master)
[![codecov](https://codecov.io/gh/appneta/go-traceview/branch/master/graph/badge.svg)](https://codecov.io/gh/appneta/go-traceview)

### Usage examples

To profile the performance of basic web service, you can use our tv.HTTPHandler wrapper.  This will
automatically propagate any distributed trace described the request headers (e.g. from TraceView's
Java, Node.js Python, Ruby, C# or Scala instruentation) through to the response headers, if one
exists.

```go
package main

import (
    "github.com/appneta/go-traceview/v1/tv"
    "math/rand"
    "net/http"
    "time"
)

func slowHandler(w http.ResponseWriter, r *http.Request) {
    time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)
}

func main() {
    http.HandleFunc("/", tv.HTTPHandler(slowHandler))
    http.ListenAndServe(":8899", nil)
}
```

For other use cases, you may need to begin or continue traces yourself. To do so, first start a
trace, and then create a series of Layer spans or Profile regions to capture the timings of
different parts of your app.

TraceView provides two ways of measuring time spent by your code: a "Layer" span can measure e.g. a
single DB query or cache request, an outgoing HTTP or RPC request, or the entire time spent within a
controller method. A "Profile" provides a named measurement of time spent inside a Layer, and is
typically used to measure a single function call or code block, e.g. to represent the time of
expensive computation(s) occuring in a Layer. Layer spans can be created as children of other
Layers, but a Profile cannot have children.

TraceView's backend identifies a Layer's type by examining the key-value pairs associated with it;
for example, if keys named "Query" and "RemoteHost" are used, the span is assumed to represent a DB
query. Our documentation on [custom
instrumentation](http://docs.appneta.com/traceview-instrumentation#extending-traceview-customizing)
describes key names that can be used to describe incoming HTTP requests, DB queries, cache/KV server
calls, outgoing RPCs, and web framework information such as controller/action names. They can be
provided as extra arguments when calling BeginLayer(), Layer.Info(), or Layer.End().

```go
func slowFunc(ctx context.Context) {
    // profile a function call (as part of a Layer)
    defer tv.BeginProfile(ctx, "slowFunc").End()
    // ... do something slow
}

func main() {
    // create trace and bind to new context
    ctx := tv.NewContext(context.Background(), tv.NewTrace("myApp"))
    // create new layer span for this trace
    l, ctxL := tv.BeginLayer(ctx, "myLayer")

    // profile a slow part of this layer
    slowFunc(ctxL)

    // Start a new span, given a parent layer
    q1L := l.BeginLayer("myDB", "Query", "SELECT * FROM tbl1", "RemoteHost", "db1.com")
    // perform a query
    q1L.End()

    // Start a new span, given a context.Context
    q2L, _ := tv.BeginLayer(ctxL, "myDB", "Query", "SELECT * FROM tbl2", "RemoteHost", "db2.com")
    // perform a query
    q2L.End()

    l.End()
    tv.EndTrace(ctx)
}
```

### Distributed tracing and context propagation

A TraceView trace is defined by a context (a globally unique ID and metadata) that is persisted
across the different hosts, processes, languages and methods that are used to serve a request. Thus
to start a new Layer span you need either a Trace or another Layer to begin from. (The Trace
interface is also the root Layer, typically created when a request is first received.) Each new
Layer is connected to its parent, so you should begin new child Layers from parents in a way that
logically matches your application; for more information [see our custom layer
documentation](http://docs.appneta.com/traceview-instrumentation#custom-layers).

The [Go blog](https://blog.golang.org/context) recommends propagating request context using the
package [golang.org/x/net/context](godoc.org/golang.org/x/net/context): "At Google, we require that Go programmers pass a
[Context](https://godoc.org/golang.org/x/net/context) parameter as the first argument to every
function on the call path between incoming and outgoing requests." Frameworks like
[Gin](https://godoc.org/github.com/gin-gonic/gin#Context) and
[Gizmo](https://godoc.org/github.com/NYTimes/gizmo/server#ContextHandler) use Context
implementations, for example. We provide helper methods that allow a Trace to be associated with a
[context.Context](https://godoc.org/golang.org/x/net/context) interface; for example, `tv.BeginLayer`
returns both a new Layer span and an associated context.

It is not required to work with context.Context to trace your app, however. You can also use just
the Trace, Layer, and Profile interfaces directly, if it better suits your application.

```go
func runQuery(ctx context.Context) {
    l, _ := tv.BeginLayer(ctx, "DBx", "Query", "SELECT * FROM tbl", "RemoteHost", "db1.com")
    // .. execute query ..
    l.End(ctx)
}

func main() {
    // create trace and bind to new context
    ctx := tv.NewContext(context.Background(), tv.NewTrace("myApp"))

    // pass the root layer context to runQuery
    runQuery(ctx)

    // end trace
    tv.EndTrace(ctx)
}
```

### Installing

To install, you should first [sign up for a TraceView account](http://www.appneta.com/products/traceview/).

Follow the instructions during signup to install the Host Agent (“tracelyzer”). This will also
install the liboboe and liboboe-dev packages on your platform.

Then, install the following:

* [Go >= 1.5](https://golang.org/dl/)
* This package: go get github.com/appneta/go-traceview/v1/tv

### Building your app with tracing

By default, no tracing occurs unless you build your app with the build tag "traceview". Without it,
calls to this API are no-ops, allowing for precise control over which deployments are traced. To
build and run with tracing enabled, you must do so on a host with TraceView's packages installed.
Once the liboboe-dev (or liboboe-devel) package is installed you can build your service as follows:

```
 $ go build -tags traceview
```

### Configuration

The `GO_TRACEVIEW_TRACING_MODE` environment variable may be set to "always", "through", or "never".
- Mode "always" is the default, and will instruct TraceView to consider sampling every inbound request for tracing.
- Mode "through" will only continue traces started upstream by inbound requests, when a trace ID is available in the request metadata (e.g. an "X-Trace" HTTP header).
- Mode "never" will disable tracing, and will neither start nor continue traces.

### Support

While we use TraceView to trace our own production Go services, this version of our Go instrumentation is currently in beta
and under active development. We welcome your feedback, issues and feature requests, and please contact us at go@appneta.com!

### Demo

If you have installed TraceView and the this package, you can run the sample “web app” included with go-traceview:

    cd $GOPATH/src/github.com/appneta/go-traceview/sample_app
    go run main.go

A web server will run on port 8899. It doesn’t do much, except wait a bit and echo back your URL path:

    $ curl http://localhost:8899/hello
    Slow request... Path: /hello

You should see these requests appear on your TraceView dashboard.  

## License

Copyright (c) 2016 Appneta, Inc.

Released under the [AppNeta Open License](http://www.appneta.com/appneta-license), Version 1.0

