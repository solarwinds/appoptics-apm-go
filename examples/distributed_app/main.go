// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/appneta/go-appneta/v1/tv"
)

var uptimeStart = time.Now()

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
	offset := time.Duration(float64(time.Millisecond) * time.Now().Sub(uptimeStart).Seconds())
	time.Sleep(normalAround(1*time.Second, 100*time.Millisecond) + offset)
	fmt.Fprintf(w, "Slowly request... Path: %s", r.URL.Path)
}

func main() {
	flag.Parse()
	if *testClients {
		go doForever("GET", "http://localhost:8899/slow")
		go doForever("POST", "http://localhost:8899/slowly")
	}
	http.HandleFunc("/slow", tv.HTTPHandler(slowHandler))
	http.HandleFunc("/slowly", tv.HTTPHandler(increasinglySlowHandler))
	http.ListenAndServe(":8899", nil)
}

var testClients = flag.Bool("testClients", false, "run test clients in separate goroutines")

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
