// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"io/ioutil"
	"net/http"

	"golang.org/x/net/context"

	"github.com/appneta/go-appneta/v1/tv"
)

func aliceHandler(w http.ResponseWriter, r *http.Request) {
	// trace this request, overwriting w with wrapped ResponseWriter
	t, w := tv.TraceFromHTTPRequestResponse("myHandler", w, r)
	ctx := tv.NewContext(context.Background(), t)

	// call an HTTP endpoint and propagate the distributed trace context
	url := "http://127.0.0.1:8891/bob"
	// begin layer for the client side of the HTTP service request
	l := tv.BeginHTTPClientLayer(ctx, r)
	defer l.End()

	// create HTTP client and set trace metadata header
	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("GET", url, nil)

	// make HTTP request to external API
	resp, err := httpClient.Do(httpReq)
	l.AddHTTPResponse(resp, err)
	if err != nil {
		l.Err(err)
	}
	// read response body
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		l.Err(err)
		w.Write([]byte(`{"error":true}`))
	} else {
		w.Write(buf) // return API response to caller
	}
}

func main() {
	http.HandleFunc("/alice", tv.HTTPHandler(aliceHandler))
	http.ListenAndServe(":8890", nil)
}
