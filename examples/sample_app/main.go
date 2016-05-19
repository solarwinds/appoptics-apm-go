// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/appneta/go-appneta/v1/tv"
)

// Our "app" doesn't do much:
func slowHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)
	fmt.Fprintf(w, "Slow request... Path: %s", r.URL.Path)
}

func main() {
	http.HandleFunc("/", tv.HTTPHandler(slowHandler))
	http.ListenAndServe(":8899", nil)
}
