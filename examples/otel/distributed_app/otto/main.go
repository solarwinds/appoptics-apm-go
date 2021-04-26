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
	ctx := r.Context()
	serverSpan := trace.SpanFromContext(ctx)
	serverSpan.SetAttributes(attribute.String("HTTP-Host", r.Host))
	serverSpan.SetAttributes(attribute.String("HTTP-URL", r.URL.String()))
	serverSpan.SetAttributes(attribute.String("HTTP-Method", r.Method))

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
			_, clientSpan := serverSpan.Tracer().Start(ctx, "ottoHTTPClient")

			// make HTTP request to external API
			resp, err := client.Do(req)
			if err != nil {
				clientSpan.RecordError(err)
				clientSpan.End() // end HTTP client timing
				serverSpan.SetAttributes(attribute.Int("HTTP-StatusCode", 500))

				w.WriteHeader(500)
				return
			}
			// read response body
			defer resp.Body.Close()
			buf, err := ioutil.ReadAll(resp.Body)
			clientSpan.End() // end HTTP client timing
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

	serverSpan.SetAttributes(attribute.Int("HTTP-StatusCode", 200))
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
