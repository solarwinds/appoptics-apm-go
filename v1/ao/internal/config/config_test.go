// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	os.Setenv("APPOPTICS_COLLECTOR", "example.com:12345")
	os.Setenv("APPOPTICS_PREPEND_DOMAIN", "true")
	os.Setenv("APPOPTICS_HISTOGRAM_PRECISION", "2")
	os.Setenv("APPOPTICS_SERVICE_KEY",
		"ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go")

	c := NewConfig()
	assert.Equal(t, "example.com:12345", c.GetCollector())
	assert.Equal(t, true, c.PrependDomain)
	assert.Equal(t, 2, c.Precision)
}
