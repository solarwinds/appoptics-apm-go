// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrappers(t *testing.T) {
	os.Unsetenv(envAppOpticsCollector)
	os.Unsetenv(envAppOpticsHistogramPrecision)
	Refresh()

	assert.NotEqual(t, nil, conf)
	assert.Equal(t, defaultGRPCCollector, GetCollector())
	assert.Equal(t, defaultHistogramPrecision, GetPrecision())

	assert.NotEqual(t, nil, ReporterOpts())
}
