# TraceView for Go

[![GoDoc](https://godoc.org/github.com/appneta/go-appneta/v1/tv?status.svg)](https://godoc.org/github.com/appneta/go-appneta/v1/tv)
[![Build Status](https://travis-ci.org/appneta/go-appneta.svg?branch=master)](https://travis-ci.org/appneta/go-appneta)
[![Coverage Status](https://coveralls.io/repos/github/appneta/go-appneta/badge.svg?branch=master)](https://coveralls.io/github/appneta/go-appneta?branch=master)
[![codecov](https://codecov.io/gh/appneta/go-appneta/branch/master/graph/badge.svg)](https://codecov.io/gh/appneta/go-appneta)

* [Introduction](#introduction)
* [Getting started](#getting-started)
* [Building your app with tracing](#building-your-app-with-tracing)
* [Demo](#demo)
* [Usage examples](#usage-examples)
* [Distributed tracing and context propagation](#distributed-tracing-and-context-propagation)
* [Configuration](#configuration)
* [Support](#support)
* [License](#license)

### Introduction

[AppNeta TraceView](http://appneta.com/products/traceview) provides distributed
tracing and code-level application performance monitoring.  This repository
provides instrumentation for Go, which allows Go-based applications to be
monitored using TraceView.

Go support is currently in beta (though we run the instrumentation to process
data in our production environment!) so please share any feedback you have; PRs welcome.


### Getting started

To get tracing, you'll need a [a (free) TraceView account](http://www.appneta.com/products/traceview/).

In the product install flow, **choose to skip the webserver and select any language.**  (You'll notice the absense of Go because we're currently in beta.)

Follow the instructions during signup to install the Host Agent (“tracelyzer”). This will also
install the liboboe and liboboe-dev packages on your platform.

Then, install the following:

* [Go >= 1.5](https://golang.org/dl/)
* This package: go get github.com/appneta/go-appneta/v1/tv

The install flow will wait for 5 traces to come in from your app.  You can
check out the [demo](#demo) app below to get a quick start.  One you have 5,
you can progress to [your account's overview
page](https://login.tv.appneta.com/overview).

### Building your app with tracing

By default, no tracing occurs unless you build your app with the build tag "traceview". Without it,
calls to this API are no-ops, allowing for precise control over which deployments are traced. To
build and run with tracing enabled, you must do so on a host with TraceView's packages installed.
Once the liboboe-dev (or liboboe-devel) package is installed you can build your service as follows:

```
 $ go build -tags traceview
```

### Demo

If you have installed TraceView and the this package, you can run the sample “web app” included with go-appneta:

    cd $GOPATH/src/github.com/appneta/go-appneta/examples/sample_app
    go run -tags traceview main.go

A web server will run on port 8899. It doesn’t do much, except wait a bit and echo back your URL path:

    $ curl http://localhost:8899/hello
    Slow request... Path: /hello

You should see these requests appear on your TraceView dashboard.


### Usage examples

To profile the performance of basic web service, you can use our
[tv.HTTPHandler](https://godoc.org/github.com/appneta/go-appneta/v1/tv#HTTPHandler) wrapper.  This
will automatically propagate any distributed trace described in the request headers (e.g. from
TraceView's Java, Node.js Python, Ruby, C# or Scala instrumentation) through to the response
headers, if one exists.

```go
package main

import (
    "github.com/appneta/go-appneta/v1/tv"
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
expensive computation(s) occurring in a Layer. Layer spans can be created as children of other
Layers, but a Profile cannot have children.

TraceView's backend identifies a Layer's type by examining the key-value pairs associated with it;
for example, if keys named "Query" and "RemoteHost" are used, the span is assumed to represent a DB
query. Our documentation on [custom
instrumentation](http://docs.appneta.com/traceview-instrumentation#extending-traceview-customizing)
describes key names that can be used to describe incoming HTTP requests, DB queries, cache/KV server
calls, outgoing RPCs, and web framework information such as controller/action names. They can be
provided as extra arguments when calling the
[BeginLayer()](https://godoc.org/github.com/appneta/go-appneta/v1/tv#BeginLayer),
[Info()](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Layer), or
[End()](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Layer) methods.

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
package [golang.org/x/net/context](godoc.org/golang.org/x/net/context): "At Google, we require that
Go programmers pass a [Context](https://godoc.org/golang.org/x/net/context) parameter as the first
argument to every function on the call path between incoming and outgoing requests." Frameworks like
[Gin](https://godoc.org/github.com/gin-gonic/gin#Context) and
[Gizmo](https://godoc.org/github.com/NYTimes/gizmo/server#ContextHandler) use Context
implementations, for example. We provide helper methods that allow a Trace to be associated with a
[context.Context](https://godoc.org/golang.org/x/net/context) interface; for example,
[tv.BeginLayer](https://godoc.org/github.com/appneta/go-appneta/v1/tv#BeginLayer) returns both a
new Layer span and an associated context, and
[tv.Info(ctx)](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Info) and
[tv.End(ctx)](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Layer) both use the Layer
defined in the provided context.

It is not required to work with context.Context to trace your app, however. You can also use just
the [Trace](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Trace),
[Layer](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Layer), and
[Profile](https://godoc.org/github.com/appneta/go-appneta/v1/tv#Profile) interfaces directly, if it
better suits your application.

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


### Configuration

The `GO_TRACEVIEW_TRACING_MODE` environment variable may be set to "always", "through", or "never".
- Mode "always" is the default, and will instruct TraceView to consider sampling every inbound request for tracing.
- Mode "through" will only continue traces started upstream by inbound requests, when a trace ID is available in the request metadata (e.g. an "X-Trace" HTTP header).
- Mode "never" will disable tracing, and will neither start nor continue traces.

### Support

While we use TraceView to trace our own production Go services, this version of our Go instrumentation is currently in beta
and under active development. We welcome your feedback, issues and feature requests, and please contact us at go@appneta.com!

## License

Copyright (c) 2016 AppNeta, Inc.

Released under the [AppNeta Open License](http://www.appneta.com/appneta-license), Version 1.0

