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
	assert.NotEqual(t, r.GetUpdateTime(), i)
}
