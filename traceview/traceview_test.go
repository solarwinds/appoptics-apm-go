package traceview

import (
	"log"
	"testing"
)

// Excercise sampling rate logic:
func TestSampleRequest(t *testing.T) {
	sampled := 0
	total := 1000
	for i := 0; i < total; i++ {
		if ok, _, _ := ShouldTraceRequest(test_layer, ""); ok {
			sampled++
		}
	}

	log.Printf("Sampled %d / %d requests", sampled, total)

	if sampled == 0 {
		t.Errorf("Expected to sample a request.")
	}
}
