// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestNullReporter(t *testing.T) {
	reporter = &nullReporter{}
	assert.False(t, reporter.IsOpen())

	// The nullReporter should seem like a regular reporter and not break
	assert.NotPanics(t, func() {
		ctx := NewContext()
		err := ctx.ReportEvent("info", test_layer, "Controller", "test_controller", "Action", "test_action")
		assert.NoError(t, err)
	})

	buf := []byte("xxx")
	cnt, err := reporter.WritePacket(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(buf), cnt)
}
