// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadEnvs(t *testing.T) {
	r := &ReporterOptions{}

	r.SetEventFlushInterval(20)
	assert.Equal(t, r.GetEventFlushInterval(), int64(20))

	r.SetEventFlushBatchSize(2000)
	assert.Equal(t, r.GetEventFlushBatchSize(), int64(2000))
}
