// OpenTracing test web app based on AppOptics demo app alice/main.go
// Wraps standard HTTP handlers with AppOptics's OpenTracing instrumentation

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"sync"

	ao "github.com/appoptics/appoptics-apm-go/v1/ao/opentelemetry"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// hard-coded service discovery
var urls = []string{
	"http://bob:8081/bob",
	"http://carol:8082/carol",
	"http://dave:8083/",
}

func ottoHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverSpan := trace.SpanFromContext(ctx)
	serverSpan.SetAttributes(attribute.String("HTTP-Host", r.Host))
	defer serverSpan.End()
	log.Printf("HTTP: %s %s", r.Method, r.URL.String())

	// call an HTTP endpoint and propagate the distributed trace context
	url := urls[rand.Intn(len(urls))]

	// create HTTP client and set trace metadata header
	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("GET", url, nil)

	log.Printf("HTTPHeaders are %v", httpReq.Header)
	ctx, clientSpan := serverSpan.Tracer().Start(ctx, "clientSpan")
	// make HTTP request to external API
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("err: %v", err)))
		clientSpan.End() // end HTTP client timing
		return
	}

	// read response body
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	clientSpan.End() // end HTTP client timing
	if err != nil {
		w.Write([]byte(`{"error":true}`))
	} else {
		w.Write(buf) // return API response to caller
	}
}

func concurrentOttoHandler(w http.ResponseWriter, r *http.Request) {
	// trace this request
	carrier := ot.HTTPHeadersCarrier(r.Header)
	wireCtx, err := ot.GlobalTracer().Extract(ot.HTTPHeaders, carrier)
	if err != nil {
		log.Printf("concurrent HTTPHeaders Extract err: %v", err)
	}
	serverSpan := ot.GlobalTracer().StartSpan("concurrentOttoHandler", ext.RPCServerOption(wireCtx))
	serverSpan.SetTag("HTTP-Host", r.Host)
	ext.HTTPUrl.Set(serverSpan, r.URL.String())
	ext.HTTPMethod.Set(serverSpan, r.Method)
	defer serverSpan.Finish()

	// call an HTTP endpoint and propagate the distributed trace context
	var wg sync.WaitGroup
	wg.Add(len(urls))
	var out []byte
	outCh := make(chan []byte)
	doneCh := make(chan struct{})
	go func() {
		for buf := range outCh {
			out = append(out, buf...)
		}
		close(doneCh)
	}()
	for _, u := range urls {
		go func(url string) {
			// create HTTP client and set trace metadata header
			client := &http.Client{}
			req, _ := http.NewRequest("GET", url, nil)
			// begin layer for the client side of the HTTP service request
			clientSpan := ot.StartSpan("ottoHTTPClient", ot.ChildOf(serverSpan.Context()))
			err = clientSpan.Tracer().Inject(clientSpan.Context(), ot.HTTPHeaders, ot.HTTPHeadersCarrier(req.Header))
			if err != nil {
				log.Printf("HTTPHeaders Inject error: %v", err)
			}

			// make HTTP request to external API
			resp, err := client.Do(req)
			if err != nil {
				clientSpan.LogFields(otlog.Error(err))
				clientSpan.Finish() // end HTTP client timing
				ext.HTTPStatusCode.Set(serverSpan, 500)
				w.WriteHeader(500)
				return
			}
			// read response body
			defer resp.Body.Close()
			buf, err := ioutil.ReadAll(resp.Body)
			clientSpan.Finish() // end HTTP client timing
			if err != nil {
				outCh <- []byte(fmt.Sprintf(`{"error":"%v"}`, err))
			} else {
				outCh <- buf
			}
			wg.Done()
		}(u)
	}
	wg.Wait()
	close(outCh)
	<-doneCh

	ext.HTTPStatusCode.Set(serverSpan, 200)
	w.Write(out)
}

func main() {
	tp, _ := ao.NewTracerProvider()
	otel.SetTracerProvider(tp)

	otto := otelhttp.NewHandler(http.HandlerFunc(ottoHandler), "Otto")
	concurrentOtto := otelhttp.NewHandler(http.HandlerFunc(concurrentOttoHandler), "ConcurrentOtto")
	http.Handle("/otto", otto)
	http.Handle("/concurrent", concurrentOtto)
	http.ListenAndServe(":8084", nil)
}
