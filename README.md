# AppOptics APM instrumentation for Go

[![GoDoc](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao?status.svg)](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao)
[![Build Status](https://travis-ci.org/appoptics/appoptics-apm-go.svg?branch=master)](https://travis-ci.org/appoptics/appoptics-apm-go)
[![Coverage Status](https://coveralls.io/repos/github/appoptics/appoptics-apm-go/badge.svg?branch=master)](https://coveralls.io/github/appoptics/appoptics-apm-go?branch=master)
[![codecov](https://codecov.io/gh/appoptics/appoptics-apm-go/branch/master/graph/badge.svg)](https://codecov.io/gh/appoptics/appoptics-apm-go)

* [Introduction](#introduction)
* [Getting started](#getting-started)
    - [Installing](#installing)
    - [Building your app with tracing](#building-your-app-with-tracing)
* [Instrumenting your application](#instrumenting-your-application)
    - [Usage examples](#usage-examples)
    - [Distributed tracing and context propagation](#distributed-tracing-and-context-propagation)
    - [Configuration](#configuration)
    - [Metrics](#metrics)
* [Help and examples](#help-and-examples)
    - [Support](#support)
    - [Demo app](#demo-app)
* [License](#license)


## Introduction

[AppOptics](http://www.appoptics.com) provides distributed tracing,
code-level application performance monitoring, and host and infrastructure monitoring. AppOptics's cross-language, production-ready,
low-overhead distributed tracing system (similar to Dapper, Zipkin, or X-Trace) can follow the path
of an application request as it is processed and forwarded between services written in [Go, Java,
Scala, Node.js, Ruby, Python, PHP, and C#/.NET](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/), reporting data to AppOptics's cloud platform for
analysis, monitoring, and fine-grained, filterable data visualization. This repository provides
instrumentation for Go, which allows Go-based applications to be monitored using AppOptics.

Go support is currently in beta (though we run the instrumentation to process
data in our production environment!) so please share any feedback you have; PRs welcome.
Also, you can read more about how we use Go at AppOptics in our [blog post announcing our Go instrumentation]()!

## Getting started

### Installing

To get tracing, you'll need a [(free) AppOptics account](http://www.appoptics.com).

In the product install flow, **choose to skip the webserver and select any language.**  (You'll notice the absence of Go because we're currently in beta.)

Follow the instructions during signup to install the Host Agent (“tracelyzer”). This will also
install the liboboe and liboboe-dev packages on your platform.

Then, install the following:

* [Go >= 1.5](https://golang.org/dl/)
* This package: go get github.com/appoptics/appoptics-apm-go/v1/ao

The install flow will wait for some data to come in from your service before continuing. You can
check out the [demo](#demo) app below to get a quick start.

### Building your app with or without tracing

No tracing occurs when you build your app with the build tag `disable_tracing`. With this tag,
calls to this API are no-ops, and [empty structs](https://dave.cheney.net/2014/03/25/the-empty-struct) keep span and instrumentation types from making memory allocations. This allows for precise control over which deployments are traced.

```
 # Set to disable tracing and APM instrumentation
 $ go build -tags disable_tracing
```

## Instrumenting your application

### Usage examples

The simplest integration option is this package's
[ao.HTTPHandler](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#HTTPHandler) wrapper.  This
will monitor the performance of the provided
[http.HandlerFunc](https://golang.org/pkg/net/http/#HandlerFunc), visualized with latency
distribution heatmaps filterable by dimensions such as URL host & path, HTTP status & method, server
hostname, etc. [ao.HTTPHandler](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#HTTPHandler)
will also continue a distributed trace described in the incoming HTTP request headers (from
AppOptics's [automatic](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/)
[C#/.NET](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/#net),
[Java](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/#java-scala),
Node.js,
[PHP](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/#php),
[Python](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/#python),
Ruby, and
[Scala](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/#java-scala) instrumentation) through to the HTTP
response.

```go
package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
)

var startTime = time.Now()

func normalAround(ts, stdDev time.Duration) time.Duration {
	return time.Duration(rand.NormFloat64()*float64(stdDev)) + ts
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(normalAround(2*time.Second, 100*time.Millisecond))
	w.WriteHeader(404)
	fmt.Fprintf(w, "Slow request, path: %s", r.URL.Path)
}

func increasinglySlowHandler(w http.ResponseWriter, r *http.Request) {
	// getting slower at a rate of 1ms per second
	offset := time.Duration(float64(time.Millisecond) * time.Now().Sub(startTime).Seconds())
	time.Sleep(normalAround(time.Second+offset, 100*time.Millisecond))
	fmt.Fprintf(w, "Slowing request, path: %s", r.URL.Path)
}

func main() {
	http.HandleFunc("/slow", ao.HTTPHandler(slowHandler))
	http.HandleFunc("/slowly", ao.HTTPHandler(increasinglySlowHandler))
	http.ListenAndServe(":8899", nil)
}
```
![sample_app screenshot](https://github.com/appoptics/appoptics-apm-go/raw/master/img/readme-screenshot1.png)

To monitor more than just the overall latency of each request to your Go service, you will need to
break a request's processing time down by placing small benchmarks into your code. To do so, first
start or continue a `Trace` (the root `Span`), then create a series of `Span`s (or `Profile`
timings) to capture the time used by different parts of the app's stack as it is processed.

AppOptics provides two ways of measuring time spent by your code: a `Span` can measure e.g. a
single DB query or cache request, an outgoing HTTP or RPC request, the entire time spent within a
controller method, or the time used by a middleware method between calls to child Spans. A
`Profile` provides a named measurement of time spent inside a `Span`, and is typically used to
measure a single function call or code block, e.g. to represent the time used by expensive
computation(s) occurring in a `Span`. Spans can be created as children of other Spans, but a
`Profile` cannot have children.

AppOptics identifies a reported Span's type from its key-value pairs; if keys named "Query" and
"RemoteHost" are used, a Span is assumed to measure the extent of a DB query. KV pairs can be
appended to Span and Profile extents as optional extra variadic arguments to methods such as
[BeginSpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginSpan) or
[BeginProfile()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginProfile),
[Info()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Span), and
[End()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Span). We also provide helper methods
such as [BeginQuerySpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginQuerySpan),
[BeginCacheSpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginCacheSpan),
[BeginRemoteURLSpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginRemoteURLSpan),
[BeginRPCSpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginRPCSpan), and
[BeginHTTPClientSpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginHTTPClientSpan)
that match the spec in our
[custom instrumentation docs](https://docs.appoptics.com/kb/apm_tracing/custom_instrumentation/)
to report attributes associated with different types of service calls, used for indexing AppOptics's
filterable charts and latency heatmaps.

```go
func slowFunc(ctx context.Context) {
    // profile a slow function call
    defer ao.BeginProfile(ctx, "slowFunc").End()
    // ... do something slow
}

func myHandler(ctx context.Context) {
    // create new ao.Span and context.Context for this part of the request
    L, ctxL := ao.BeginSpan(ctx, "myHandler")
    defer L.End()

    // profile a slow part of this ao.Span
    slowFunc(ctxL)

    // Start a new Span, given a parent span
    mL, _ := L.BeginSpan("myMiddleware")
    // ... do something slow ...
    mL.End()

    // Start a new query Span, given a context.Context
    q1L := ao.BeginQuerySpan(ctxL, "myDB", "SELECT * FROM tbl1", "postgresql", "db1.com")
    // perform a query
    q1L.End()
}

func processRequest() {
    // Create ao.Trace and bind to new context.Context
    ctx := ao.NewContext(context.Background(), ao.NewTrace("myApp"))
    myHandler(ctx) // Dispatch handler for request
    ao.EndTrace(ctx)
}
```

### Retrieving the context from an http request

A common pattern when tracing in golang is to call `ao.HTTPHandler(handler)` then retrieve the trace
context inside of the handler.  
```go

func myHandler( w http.ResponseWriter, r *http.Request ) {
    // trace this request, overwriting w with wrapped ResponseWriter
    t, w := ao.TraceFromHTTPRequestResponse("myHandler", w, r)
    ctx := ao.NewContext(context.Background(), t)
    defer t.End()

    // create a query span
    q1L := ao.BeginQuerySpan(ctx, "myDB", "SELECT * FROM tbl1", "postgresql", "db1.com")

    // run your query here

    // end the query span
    q1L.End()

    fmt.Fprintf(w, "I ran a query")

func main() {
    http.HandleFunc("/endpoint", ao.HTTPHandler(myHandler))
    http.ListenAndServe(":8899", nil)
}
```

### Distributed tracing and context propagation

A AppOptics trace is defined by a context (a globally unique ID and metadata) that is persisted
across the different hosts, processes, languages and methods that are used to serve a request. Thus
to start a new Span you need either a Trace or another Span to begin from. (The Trace
interface is also the root Span, typically created when a request is first received.) Each new
Span is connected to its parent, so you should begin new child Spans from parents in a way that
logically matches your application; for more information [see our custom span
documentation](https://docs.appoptics.com/kb/apm_tracing/custom_instrumentation/).

The [Go blog](https://blog.golang.org/context) recommends propagating request context using the
package [golang.org/x/net/context](godoc.org/golang.org/x/net/context): "At Google, we require that
Go programmers pass a [Context](https://godoc.org/golang.org/x/net/context) parameter as the first
argument to every function on the call path between incoming and outgoing requests." Frameworks like
[Gin](https://godoc.org/github.com/gin-gonic/gin#Context) and
[Gizmo](https://godoc.org/github.com/NYTimes/gizmo/server#ContextHandler) use Context
implementations, for example. We provide helper methods that allow a Trace to be associated with a
[context.Context](https://godoc.org/golang.org/x/net/context) interface; for example,
[ao.BeginSpan](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginSpan) returns both a
new Span and an associated context, and
[ao.Info(ctx)](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Info) and
[ao.End(ctx)](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Span) both use the Span
defined in the provided context.

It is not required to work with context.Context to trace your app, however. You can also use just
the [Trace](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Trace),
[Span](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Span), and
[Profile](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Profile) interfaces directly, if it
better suits your instrumentation use case.

```go
func runQuery(ctx context.Context, query string) {
    l := ao.BeginQuerySpan(ctx, "myDB", query, "postgresql", "db1.com")
    // .. execute query ..
    l.End()
}

func main() {
    // create trace and bind to new context
    ctx := ao.NewContext(context.Background(), ao.NewTrace("myApp"))

    // pass the root span context to runQuery
    runQuery(ctx)

    // end trace
    ao.EndTrace(ctx)
}
```


### Configuration

The `APPOPTICS_TRACING_MODE` environment variable may be set to "always" or "never".
- Mode "always" is the default, and will instruct AppOptics to consider sampling every inbound request for tracing.
- Mode "never" will disable tracing, and will neither start nor continue traces.


### Metrics

In addition to distributed tracing and latency measurement, this package also provides Go metrics
monitoring for runtime metrics such as memory and number of goroutines. Below is a screenshot from
our [blog's release announcement]() of a latency
heatmap underlaid with memory metrics, just after fixing a memory leak and restarting a service
inside a staging environment.

<img width=729 src="https://github.com/appoptics/appoptics-apm-go/raw/master/img/metrics-screenshot1.png">

## Help and examples

### Support

While we use AppOptics to trace our own production Go services, this version of our Go instrumentation is currently in beta
and under active development. We welcome your feedback, issues and feature requests, and please contact us at support@appoptics.com!

### Demo web app

If you have installed AppOptics and this package, you can run the sample “web app” included with appoptics-apm-go:

    $ cd $GOPATH/src/github.com/appoptics/appoptics-apm-go/examples/sample_app
    $ go run main.go

A web server will run on port 8899. It doesn’t do much, except wait a bit and echo back your URL path:

    $ curl http://localhost:8899/hello
    Slow request... Path: /hello

You should see these requests appear on your AppOptics dashboard.

### Distributed app

There is also a demonstration of distributed tracing in examples/distributed_app, a sample system
comprised of two Go services, a Node.js service, and a Python service. It can be built and run from
source in each service's subdirectory, or by using docker-compose:

    $ cd $GOPATH/src/github.com/appoptics/appoptics-apm-go/examples/distributed_app
    $ docker-compose build
    # ... (building)
    $ APPOPTICS_SERVICE_KEY=xxx docker-compose up -d
    Starting distributedapp_alice_1
    Starting distributedapp_bob_1
    Starting distributedapp_caroljs_1
    Starting distributedapp_davepy_1
    Starting distributedapp_redis_1

and substituting "xxx" with your AppOptics access key. This app currently provides two HTTP handlers:
`aliceHandler`, which randomly forwards requests to either of "bob" (Go), "caroljs", or "davepy",
and `concurrentAliceHandler`, which makes requests to all three in parallel.

    $ curl http://localhost:8890/alice
    {"result":"hello from bob"}
    $ curl http://localhost:8890/alice
    Hello from Flask!
    $ curl http://localhost:8890/alice
    Hello from app.js
    $ curl http://localhost:8890/concurrent
    Hello from Flask!{"result":"hello from bob"}Hello from app.js

You should see traces for these appear on your AppOptics dashboard. Here is an example trace of the
concurrent handler:

<img width=729 src="https://github.com/appoptics/appoptics-apm-go/raw/master/img/concurrent-tracedetails.gif">

### OpenTracing

Support for the OpenTracing 1.0 API is available in the [opentracing](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao/opentracing) package as a technology preview. The
OpenTracing tracer in that package provides support for OpenTracing span reporting and context
propagation by using AppOptics's span reporting and HTTP header formats, permitting the OT tracer
to continue distributed traces started by AppOptics's instrumentation and vice versa. Some of the
OpenTracing standardized tags are mapped to AppOptics tag names as well.

To set AppOptics's tracer to be your global tracer, call `opentracing.InitGlobalTracer(opentracing.NewTracer())`.
Currently, `opentracing.NewTracer()` does not accept any options, but this may change in the future.
Please let us know if you are using this package while it is in preview by contacting us at opentracing@tracelytics.com.

## License

Copyright (c) 2017 Librato, Inc.

Released under the [Librato Open License](http://docs.traceview.solarwinds.com/Instrumentation/librato-open-license.html), Version 1.0

