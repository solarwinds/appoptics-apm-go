# AppOptics APM instrumentation for Go

[![GoDoc](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao?status.svg)](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao)
[![Build Status](https://travis-ci.org/appoptics/appoptics-apm-go.svg?branch=master)](https://travis-ci.org/appoptics/appoptics-apm-go)
[![codecov](https://codecov.io/gh/appoptics/appoptics-apm-go/branch/master/graph/badge.svg)](https://codecov.io/gh/appoptics/appoptics-apm-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/appoptics/appoptics-apm-go)](https://goreportcard.com/report/github.com/appoptics/appoptics-apm-go)

* [Introduction](#introduction)
* [Getting started](#getting-started)
    - [Prerequisites](#prerequisites)
    - [Installing](#installing)
    - [Running your app with or without tracing](#running-your-app-with-or-without-tracing)
* [Instrumenting your application](#instrumenting-your-application)
    - [Usage examples](#usage-examples)
    - [Distributed tracing and context propagation](#distributed-tracing-and-context-propagation)
    - [Configuration](#configuration)
* [Help and examples](#help-and-examples)
    - [Support](#support)
    - [Demo web app](#demo-web-app)
    - [Distributed app](#distributed-app)
    - [OpenTracing](#opentracing)
* [License](#license)


## Introduction

[AppOptics](http://www.appoptics.com) is SaaS-based monitoring with distributed
tracing, code-level application performance monitoring, and host and
infrastructure monitoring.  This repository provides instrumentation for Go,
which allows Go-based applications to be monitored using AppOptics.

AppOptics's cross-language, production-ready,
low-overhead distributed tracing system (similar to Dapper, Zipkin, or X-Trace) can follow the path
of an application request as it is processed and forwarded between services written in [Go, Java,
Scala, Node.js, Ruby, Python, PHP, and C#/.NET](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/), reporting data to AppOptics's cloud platform for
analysis, monitoring, and fine-grained, filterable data visualization.

We run this in our production environment, as do our customers.  Please share any feedback you have; PRs welcome.

## Getting started

### Prerequisites

To get tracing, you'll need a [(free) AppOptics account](http://www.appoptics.com).  Follow the instructions during signup to install the Go Agent.

The in-app install flow will wait for some data to come in from your service before continuing. You can either instrument your application (read on), or 
check out the [demo](#demo) app below to get a quick start!

### Minimum Supported Go Version

* [Go >= 1.11](https://golang.org/dl/)

### Installing

Install the following to get started:

* This package: go get github.com/appoptics/appoptics-apm-go/v1/ao

Note that the service key needs to be configured for a successful setup. See [Configuration](#configuration) 
for more info.


### Running your app with or without tracing

No tracing occurs when the environment variable `APPOPTICS_DISABLED` is explicitly set to `true`. With this environment variable,
calls to the APIs are no-ops, and [empty structs](https://dave.cheney.net/2014/03/25/the-empty-struct) keep span and instrumentation types from making memory allocations. This allows for precise control over which deployments are traced.

```
 # Set to disable tracing and APM instrumentation
 $ export APPOPTICS_DISABLED=true
```

## Instrumenting your application

### Usage examples

The simplest integration option is this package's
[ao.HTTPHandler](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#HTTPHandler) wrapper.  This
will monitor the performance of the provided
[http.HandlerFunc](https://golang.org/pkg/net/http/#HandlerFunc), visualized with latency
distribution heatmaps filterable by dimensions such as URL host & path, HTTP status & method, server
hostname, etc. [ao.HTTPHandler](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#HTTPHandler)
will also continue a distributed trace described in the incoming HTTP request headers (from either another instrumented Golang application or 
AppOptics's automatic instrumentation of [our other supported application runtimes](https://docs.appoptics.com/kb/apm_tracing/supported_platforms/)).

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
![sample_app screenshot](https://github.com/appoptics/appoptics-apm-go/raw/master/img/readme-ao-screenshot1.png)
![sample_app screenshot2](https://github.com/appoptics/appoptics-apm-go/raw/master/img/readme-ao-screenshot2.png)

To monitor more than just the overall latency of each request to your Go service, you will need to
break a request's processing time down by placing small benchmarks into your code. To do so, first
start or continue a `Trace` (the root `Span`), then create a series of `Span`s to capture the time used by different parts of the app's stack as it is processed.

AppOptics provides a way to measure time spent by your code: a `Span` can measure e.g. a
single DB query or cache request, an outgoing HTTP or RPC request, the entire time spent within a
controller method, or the time used by a middleware method between calls to child Spans. Spans can be created
 as children of other Spans.

AppOptics identifies a reported Span's type from its key-value pairs; if keys named "Query" and
"RemoteHost" are used, a Span is assumed to measure the extent of a DB query. KV pairs can be
appended to Spans as optional extra variadic arguments to methods such as
[BeginSpan()](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginSpan) or
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
func myHandler(ctx context.Context) {
    // create new ao.Span and context.Context for this part of the request
    L, ctxL := ao.BeginSpan(ctx, "myHandler")
    defer L.End()

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
    t, w, r := ao.TraceFromHTTPRequestResponse("myHandler", w, r)
    ctx := ao.NewContext(context.Background(), t)
    defer t.End()

    // create a query span
    q1L := ao.BeginQuerySpan(ctx, "myDB", "SELECT * FROM tbl1", "postgresql", "db1.com")

    // run your query here

    // end the query span
    q1L.End()

    fmt.Fprintf(w, "I ran a query")
}

func main() {
    http.HandleFunc("/endpoint", ao.HTTPHandler(myHandler))
    http.ListenAndServe(":8899", nil)
}
```

### Custom transaction names

Our out-of-the-box instrumentation assigns transaction name based on URL and Controller/Action values detected. However, you may want to override the transaction name to better describe your instrumented operation. Take note that transaction name is converted to lowercase, and might be truncated with invalid characters replaced.

If multiple transaction names are set on the same trace, then the last one would be used.

Empty string and null are considered invalid transaction name values and will be ignored.

#### Set a custom transaction name from the HTTP handler

`ao.SetTransactionName(ctx context.Context, name string)` is used to set the custom transaction name in
the HTTP handler. The current http.Request context is needed to retrieve the AppOptics tracing context, if any.

```go
func slowHandler(w http.ResponseWriter, r *http.Request) {
    ao.SetTransactionName(r.Context(), "my-custom-transaction-name")
    time.Sleep(normalAround(2*time.Second, 100*time.Millisecond))
    w.WriteHeader(404)
    fmt.Fprintf(w, "Slow request... Path: %s", r.URL.Path)
}
```

#### Set a custom transaction name from SDK

When you use AppOptics SDK to create the Span by yourself, the `SetTransactionName(string)` method can be invoked
to set the custom transaction name.

```go
    ...
    // create new ao.Span and context.Context for this part of the request
    L, ctxL := ao.BeginSpan(ctx, "myHandler")
    L.SetTransactionName("my-custom-transaction-name")
    defer L.End()
    ...
```

You can also set the environment variable `APPOPTICS_PREPEND_DOMAIN` to `true` if you need to
prepend the hostname to the transaction name. This works for both default transaction names and
the custom transaction names provided by you.

### Distributed tracing and context propagation

An AppOptics trace is defined by a context (a globally unique ID and metadata) that is persisted
across the different hosts, processes, languages and methods that are used to serve a request. Thus
to start a new Span you need either a Trace or another Span to begin from. (The Trace
interface is also the root Span, typically created when a request is first received.) Each new
Span is connected to its parent, so you should begin new child Spans from parents in a way that
logically matches your application; for more information [see our custom span
documentation](https://docs.appoptics.com/kb/apm_tracing/custom_instrumentation/).

The [Go blog](https://blog.golang.org/context) recommends propagating request context using the
package [context](godoc.org/context): "At Google, we require that
Go programmers pass a [Context](https://godoc.org/context) parameter as the first
argument to every function on the call path between incoming and outgoing requests." Frameworks like
[Gin](https://godoc.org/github.com/gin-gonic/gin#Context) and
[Gizmo](https://godoc.org/github.com/NYTimes/gizmo/server#ContextHandler) use Context
implementations, for example. We provide helper methods that allow a Trace to be associated with a
[context.Context](https://godoc.org/context) interface; for example,
[ao.BeginSpan](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#BeginSpan) returns both a
new Span and an associated context, and
[ao.Info(ctx)](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Info) and
[ao.End(ctx)](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Span) both use the Span
defined in the provided context.

It is not required to work with context.Context to trace your app, however. You can also use just
the [Trace](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Trace) and
[Span](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao#Span) interfaces directly, if it
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

These environment variables may be set:

| Variable Name        | Required           | Default  | Description |
| -------------------- | ------------------ | -------- | ----------- |
|APPOPTICS_SERVICE_KEY|Yes||The service key identifies the service being instrumented within your Organization. It should be in the form of ``<api token>:<service name>``.|
|APPOPTICS_DEBUG_LEVEL|No|WARN|Logging level to adjust the logging verbosity. Increase the logging verbosity to one of the debug levels to get more detailed information. Possible values: DEBUG, INFO, WARN, ERROR|
|APPOPTICS_HOSTNAME_ALIAS|No||A logical/readable hostname that can be used to easily identify the host|
|APPOPTICS_TRACING_MODE|No|enabled|Mode "enabled" will instruct AppOptics to consider sampling every inbound request for tracing. Mode "disabled" will disable tracing, and will neither start nor continue traces.|
|APPOPTICS_REPORTER|No|ssl|The reporter that will be used throughout the runtime of the app. Possible values: ssl, udp, none|
|APPOPTICS_COLLECTOR|No|collector.appoptics.com:443|SSL collector endpoint address and port (only used if APPOPTICS_REPORTER = ssl).|
|APPOPTICS_COLLECTOR_UDP|No|127.0.0.1:7831|UDP collector endpoint address and port (only used if APPOPTICS_REPORTER = udp).|
|APPOPTICS_TRUSTEDPATH|No||Path to the certificate used to verify the collector endpoint.|
|APPOPTICS_INSECURE_SKIP_VERIFY|No|true|Skip verification of the server's certificate chain and host name. Possible values: true, false|
|APPOPTICS_PREPEND_DOMAIN|No|false|Prepend the domain name to the transaction name. Possible values: true, false|
|APPOPTICS_DISABLED|No|false|Disable the agent. Possible values: true, false|
|APPOPTICS_EC2_METADATA_TIMEOUT|No|1000|EC2 metadata retrieval timeout value in milliseconds. Setting to 0 effectively disables fetching from the metadata URL (not waiting), should only be used if not running on EC2 / Openstack and need to minimize agent start up time. Possible values: [0, 3000]|

For the up-to-date configuration items and descriptions, including YAML config file support in the upcoming version, please refer to our knowledge base website: https://docs.appoptics.com/kb/apm_tracing/go/configure/

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
comprised of three Go services and two Python services. It can be built and run from
source in each service's subdirectory, or by using docker-compose:

    $ cd $GOPATH/src/github.com/appoptics/appoptics-apm-go/examples/distributed_app
    $ docker-compose build
    # ... (building)
    $ APPOPTICS_API_TOKEN=xxx docker-compose up -d
    Starting distributedapp_alice_1
    Starting distributedapp_bob_1
    Starting distributedapp_caroljs_1
    Starting distributedapp_davepy_1
    Starting distributedapp_redis_1

and substituting "xxx" with your AppOptics API token.  Note that because this spins up multiple services, the service name is provided in the `docker-compose.yml` file.

This app currently provides two HTTP handlers:
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

<img width=729 src="https://github.com/appoptics/appoptics-apm-go/raw/master/img/readme-ao-screenshot3.png">

### OpenTracing

Support for the OpenTracing 1.0 API is available in the [opentracing](https://godoc.org/github.com/appoptics/appoptics-apm-go/v1/ao/opentracing) package as a technology preview. The
OpenTracing tracer in that package provides support for OpenTracing span reporting and context
propagation by using AppOptics's span reporting and HTTP header formats, permitting the OT tracer
to continue distributed traces started by AppOptics's instrumentation and vice versa. Some of the
OpenTracing standardized tags are mapped to AppOptics tag names as well.

To set AppOptics's tracer to be your global tracer, use something like the following:

```go
import(
  stdopentracing "github.com/opentracing/opentracing-go"
  aotracing "github.com/appoptics/appoptics-apm-go/v1/ao/opentracing"
)

// tracer returns the global tracing construct for OpenTracing.
// it is returned here so it can be used in functions that expect
// a Tracer, such as server init functions in go-kit
func tracer() stdopentracing.Tracer {
	tracer := aotracing.NewTracer()
	stdopentracing.SetGlobalTracer(tracer)
	return tracer
}
```

Currently, `opentracing.NewTracer()` does not accept any options, but this may change in the future.
Please let us know if you are using this package while it is in preview by contacting us at support@appoptics.com.

## License

Copyright (c) 2018 Librato, Inc.

Released under the [Librato Open License](https://docs.appoptics.com/kb/apm_tracing/librato-open-license/), Version 1.0
