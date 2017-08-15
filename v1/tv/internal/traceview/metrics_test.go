// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFlushBSON(t *testing.T) {
	r := SetGRPCTestReporter()
	assert.IsType(t, &grpcReporter{}, r)
}
