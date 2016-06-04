// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"net/http"

	"github.com/appneta/go-appneta/v1/tv"
)

func bobHandler(w http.ResponseWriter, r *http.Request) {
	t, w := tv.TraceFromHTTPRequestResponse("bobHandler", w, r)
	defer t.End()
	w.Write([]byte(`{"result":"hello from bob"}`))
}

func main() {
	http.HandleFunc("/bob", tv.HTTPHandler(bobHandler))
	http.ListenAndServe(":8081", nil)
}
