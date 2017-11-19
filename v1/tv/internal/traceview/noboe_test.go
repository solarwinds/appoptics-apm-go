// +build disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoboe(t *testing.T) {
	// no tracing if build tag not enabled
	sampled, _, _ := shouldTraceRequest("test", "")
	assert.False(t, sampled)

	r := SetTestReporter()
	sampled, _, _ = shouldTraceRequest("test", "")
	assert.True(t, sampled)
	r.ShouldTrace = false
	sampled, _, _ = shouldTraceRequest("test", "")
	assert.False(t, sampled)
}
