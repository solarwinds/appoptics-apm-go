// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"net/http"

	"github.com/appneta/go-appneta/examples/distributed_app"
	"github.com/appneta/go-appneta/v1/tv"
)

func main() {
	http.HandleFunc("/bob", tv.HTTPHandler(app.BobHandler))
	http.ListenAndServe(":8081", nil)
}
