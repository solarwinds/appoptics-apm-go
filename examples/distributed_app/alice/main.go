// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"sync"

	"golang.org/x/net/context"

	"github.com/librato/go-traceview/v1/tv"
)

// hard-coded service discovery
var urls = []string{
	"http://bob:8081/bob",
	"http://carol:8082/carol",
	"http://dave:8083/",
	"http://otto:8084/otto",
}

func aliceHandler(w http.ResponseWriter, r *http.Request) {
	// trace this request, overwriting w with wrapped ResponseWriter
	t, w := tv.TraceFromHTTPRequestResponse("aliceHandler", w, r)
	ctx := tv.NewContext(context.Background(), t)
	defer t.End()
	log.Printf("%s %s", r.Method, r.URL)

	// call an HTTP endpoint and propagate the distributed trace context
	url := urls[rand.Intn(len(urls))]

	// create HTTP client and set trace metadata header
	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("GET", url, nil)
	// begin layer for the client side of the HTTP service request
	l := tv.BeginHTTPClientLayer(ctx, httpReq)

	// make HTTP request to external API
	resp, err := httpClient.Do(httpReq)
	l.AddHTTPResponse(resp, err)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("err: %v", err)))
		l.End() // end HTTP client timing
		return
	}

	// read response body
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	l.End() // end HTTP client timing
	//w.WriteHeader(200)
	if err != nil {
		w.Write([]byte(`{"error":true}`))
	} else {
		w.Write(buf) // return API response to caller
	}
}

func concurrentAliceHandler(w http.ResponseWriter, r *http.Request) {
	// trace this request, overwriting w with wrapped ResponseWriter
	t, w := tv.TraceFromHTTPRequestResponse("aliceHandler", w, r)
	ctx := tv.NewContext(context.Background(), t)
	t.SetAsync(true)
	defer t.End()

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
			l := tv.BeginHTTPClientLayer(ctx, req)

			// make HTTP request to external API
			resp, err := client.Do(req)
			l.AddHTTPResponse(resp, err)
			if err != nil {
				l.End() // end HTTP client timing
				w.WriteHeader(500)
				return
			}
			// read response body
			defer resp.Body.Close()
			buf, err := ioutil.ReadAll(resp.Body)
			l.End() // end HTTP client timing
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

	w.Write(out)
}

func main() {
	http.HandleFunc("/alice", aliceHandler)
	http.HandleFunc("/concurrent", concurrentAliceHandler)
	http.ListenAndServe(":8890", nil)
}
