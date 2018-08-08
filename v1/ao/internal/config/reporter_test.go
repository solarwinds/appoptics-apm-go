// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadEnvs(t *testing.T) {
	r := defaultReporterOptions()
	i := r.GetUpdateTime()

	r.SetEventFlushInterval(20)
	assert.Equal(t, r.GetEventFlushInterval(), int64(20))
	j := r.GetUpdateTime()
	assert.NotEqual(t, j, i)

	r.SetEventBatchSize(3145728)
	assert.Equal(t, r.GetEventBatchSize(), int64(3145728))
	assert.NotEqual(t, r.GetUpdateTime(), j)
}
