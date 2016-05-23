// Test web app
// Wraps a standard HTTP handler with TraceView instrumentation

package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/appneta/go-appneta/v1/tv"
)

var uptimeStart = time.Now()

func normalAround(ts, stdDev time.Duration) time.Duration {
	ret := time.Duration(rand.NormFloat64()*float64(stdDev)) + ts
	return ret
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(normalAround(2*time.Second, 100*time.Millisecond))
	fmt.Fprintf(w, "Slow request... Path: %s", r.URL.Path)
	w.WriteHeader(404)
}

func increasinglySlowHandler(w http.ResponseWriter, r *http.Request) {
	// getting slower at a rate of 1ms per second
	offset := time.Duration(float64(time.Millisecond) * time.Now().Sub(uptimeStart).Seconds())
	time.Sleep(normalAround(1*time.Second, 100*time.Millisecond) + offset)
	fmt.Fprintf(w, "Slowly request... Path: %s", r.URL.Path)
}

func main() {
	go testClients()
	http.HandleFunc("/slow", tv.HTTPHandler(slowHandler))
	http.HandleFunc("/slowly", tv.HTTPHandler(increasinglySlowHandler))
	http.ListenAndServe(":8899", nil)
}

func getForever(method, url string) {
	for {
		ctx := context.Background()
		// ctx not associated with a trace, so no tracing
		resp, err := testClient(ctx, method, url)
		log.Printf("test slow resp: %v err: %v", resp, err)
	}
}
func testClients() {
	go getForever("GET", "http://localhost:8899/slow")
	go getForever("POST", "http://localhost:8899/slowly")
}

func testClient(ctx context.Context, method, url string) (*http.Response, error) {
	l, _ := tv.BeginLayer(ctx, "http.Client", "IsService", true, "RemoteURL", url)

	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest(method, url, nil)
	httpReq.Header["X-Trace"] = []string{l.MetadataString()}

	log.Printf("httpClient.Do req: %v", httpReq)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		l.Err(err)
	}
	defer resp.Body.Close()

	var endArgs []interface{}
	if resp != nil {
		endArgs = append(endArgs, "Edge", resp.Header["X-Trace"])
	}
	l.End()

	return resp, err
}
