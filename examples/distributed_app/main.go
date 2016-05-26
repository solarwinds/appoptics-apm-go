// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"io/ioutil"
	"net/http"

	"github.com/appneta/go-appneta/v1/tv"
)

func aliceHandler(w http.ResponseWriter, r *http.Request) {
	// trace this request, overwriting w with wrapped ResponseWriter
	t, w := tv.TraceFromHTTPRequestResponse("myHandler", w, r)

	// call an HTTP endpoint and propagate the distributed trace context
	url := "http://127.0.0.1:8891/bob"
	// begin layer for the client side of the HTTP service request
	l := t.BeginLayer("http.Client", "IsService", true, "RemoteURL", url)

	// create HTTP client and set trace metadata header
	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("GET", url, nil)
	httpReq.Header.Set("X-Trace", l.MetadataString())
	// make HTTP request to external API
	resp, err := httpClient.Do(httpReq)
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

	// TODO also test when no X-Trace header in response, or req fails
	l.End("Edge", resp.Header.Get("X-Trace"))
}

func main() {
	http.HandleFunc("/alice", tv.HTTPHandler(aliceHandler))
	http.ListenAndServe(":8890", nil)
}
