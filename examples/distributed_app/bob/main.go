// Test web app
// Wraps a standard HTTP handler with AppOptics instrumentation

package main

import (
	"log"
	"net/http"

	"github.com/appoptics/appoptics-apm-go/v1/tv"
)

func bobHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL)
	w.Write([]byte(`{"result":"hello from bob"}`))
}

func main() {
	http.HandleFunc("/bob", tv.HTTPHandler(bobHandler))
	http.ListenAndServe(":8081", nil)
}
