// Test web app
// Wraps a standard HTTP handler with AppOptics instrumentation

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/librato/go-traceview/v1/tv"
)

var startTime = time.Now()

func normalAround(ts, stdDev time.Duration) time.Duration {
	return time.Duration(rand.NormFloat64()*float64(stdDev)) + ts
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(normalAround(2*time.Second, 100*time.Millisecond))
	w.WriteHeader(404)
	fmt.Fprintf(w, "Slow request... Path: %s", r.URL.Path)
}

func increasinglySlowHandler(w http.ResponseWriter, r *http.Request) {
	// getting slower at a rate of 1ms per second
	offset := time.Duration(float64(time.Millisecond) * time.Now().Sub(startTime).Seconds())
	time.Sleep(normalAround(time.Second+offset, 100*time.Millisecond))
	fmt.Fprintf(w, "Slowly request... Path: %s", r.URL.Path)
}

func main() {
	http.HandleFunc("/slow", tv.HTTPHandler(slowHandler))
	http.HandleFunc("/slowly", tv.HTTPHandler(increasinglySlowHandler))
	http.ListenAndServe(addr, nil)
}

var addr string

func init() {
	var testClients bool
	flag.StringVar(&addr, "addr", ":8899", "listen address")
	flag.BoolVar(&testClients, "testClients", false, "run test clients")
	flag.Parse()
	if testClients {
		go doForever("GET", fmt.Sprintf("http://localhost%s/slow", addr))
		go doForever("POST", fmt.Sprintf("http://localhost%s/slowly", addr))
	}
}

func doForever(method, url string) {
	for {
		httpClient := &http.Client{}
		httpReq, _ := http.NewRequest(method, url, nil)
		log.Printf("httpClient.Do req: %s %s", method, url)
		resp, err := httpClient.Do(httpReq)
		if err != nil {
			log.Printf("httpClient.Do err: %v", err)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		log.Printf("httpClient.Do body: %v, err: %v", string(body), err)
	}
}
