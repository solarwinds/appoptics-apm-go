// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	key1 := "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"
	key2 := "bbbb315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"

	os.Setenv(envAppOpticsCollector, "example.com:12345")
	os.Setenv(envAppOpticsPrependDomain, "true")
	os.Setenv(envAppOpticsHistogramPrecision, "2")
	os.Setenv(envAppOpticsServiceKey, key1)

	c := NewConfig()
	assert.Equal(t, "example.com:12345", c.GetCollector())
	assert.Equal(t, true, c.PrependDomain)
	assert.Equal(t, 2, c.Precision)

	os.Setenv(envAppOpticsCollector, "test.abc:8080")
	c.RefreshConfig()
	assert.Equal(t, "test.abc:8080", c.GetCollector())

	c = NewConfig(
		WithCollector("hello.world"),
		WithServiceKey(key2))
	assert.Equal(t, "hello.world", c.GetCollector())
	assert.Equal(t, key2, c.GetServiceKey())

	os.Setenv(envAppOpticsServiceKey, key1)
	os.Setenv(envAppOpticsHostnameAlias, "test")
	os.Setenv(envAppOpticsInsecureSkipVerify, "false")
	os.Setenv(envAppOpticsTrustedPath, "test.crt")
	os.Setenv(envAppOpticsCollectorUDP, "hello.udp")

	c.RefreshConfig()
	assert.Equal(t, key1, c.GetServiceKey())
	assert.Equal(t, "test", c.GetHostAlias())
	assert.Equal(t, false, c.GetSkipVerify())
	assert.Equal(t, "test.crt", filepath.Base(c.GetTrustedPath()))
	assert.Equal(t, "hello.udp", c.GetCollectorUDP())
}
