// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"

	"golang.org/x/net/context"

	"github.com/appneta/go-appneta/v1/tv"
)

func aliceHandler(w http.ResponseWriter, r *http.Request) {
	// trace this request, overwriting w with wrapped ResponseWriter
	t, w := tv.TraceFromHTTPRequestResponse("aliceHandler", w, r)
	ctx := tv.NewContext(context.Background(), t)
	defer t.End()
	log.Printf("%s %s", r.Method, r.URL)

	// call an HTTP endpoint and propagate the distributed trace context
	urls := []string{
		"http://bob:8081/bob",
		"http://carol:8082/carol",
		"http://dave:8083/",
	}
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

func main() {
	http.HandleFunc("/alice", tv.HTTPHandler(aliceHandler))
	http.ListenAndServe(":8890", nil)
}
