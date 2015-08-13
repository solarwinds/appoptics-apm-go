// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/appneta/go-traceview/traceview"
)

// Our "app" doesn't do much:
func slow_handler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)
	fmt.Fprintf(w, "Slow request... Path: %s", r.URL.Path)
}

func main() {
	http.HandleFunc("/", traceview.InstrumentedHttpHandler(slow_handler))
	http.ListenAndServe(":8899", nil)
}
