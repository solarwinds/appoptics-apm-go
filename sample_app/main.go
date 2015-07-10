// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"fmt"
	"github.com/tracelytics/go-traceview/traceview"
	"math/rand"
	"net/http"
	"time"
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
