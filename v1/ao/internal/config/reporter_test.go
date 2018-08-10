// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadEnvs(t *testing.T) {
	r := defaultReporterOptions()

	r.SetEventFlushInterval(20)
	assert.Equal(t, r.GetEventFlushInterval(), int64(20))

	r.SetEventBatchSize(2000)
	assert.Equal(t, r.GetEventBatchSize(), int64(2000))
}
