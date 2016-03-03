// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"log"
	"testing"
)

// Excercise sampling rate logic:
func TestSampleRequest(t *testing.T) {
	_ = SetTestReporter() // set up test reporter
	sampled := 0
	total := 1000
	for i := 0; i < total; i++ {
		if ok, _, _ := shouldTraceRequest(test_layer, ""); ok {
			sampled++
		}
	}

	log.Printf("Sampled %d / %d requests", sampled, total)

	if sampled == 0 {
		t.Errorf("Expected to sample a request.")
	}
}
