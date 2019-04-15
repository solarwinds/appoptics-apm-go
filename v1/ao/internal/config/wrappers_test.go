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
	Load()

	assert.NotEqual(t, nil, conf)
	assert.Equal(t, getFieldDefaultValue(&Config{}, "Collector"), GetCollector())
	assert.Equal(t, ToInteger(getFieldDefaultValue(&Config{}, "Precision")), GetPrecision())

	assert.NotEqual(t, nil, ReporterOpts())
}
