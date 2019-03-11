// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadString(t *testing.T) {
	var envName = "TEST_STRING"

	os.Setenv(envName, "hello")
	assert.Equal(t, "hello", Env("TEST_STRING").ToString("fallback"))

	os.Setenv(envName, "")
	assert.Equal(t, "fallback", Env("TEST_STRING").ToString("fallback"))

	os.Unsetenv(envName)
	assert.Equal(t, "fallback", Env("TEST_STRING").ToString("fallback"))
}

func TestLoadBool(t *testing.T) {
	var envName = "TEST_BOOL"
	os.Setenv(envName, "true")
	assert.Equal(t, true, Env(envName).ToBool(false))

	os.Setenv(envName, "false")
	assert.Equal(t, false, Env(envName).ToBool(false))

	os.Setenv(envName, "yes")
	assert.Equal(t, true, Env(envName).ToBool(false))

	os.Setenv(envName, "no")
	assert.Equal(t, false, Env(envName).ToBool(false))

	os.Setenv(envName, "123")
	assert.Equal(t, false, Env(envName).ToBool(false))

	os.Setenv(envName, "True")
	assert.Equal(t, true, Env(envName).ToBool(false))
}

func TestLoadInt(t *testing.T) {
	var envName = "TEST_INT"
	os.Setenv(envName, "123")
	assert.Equal(t, 123, Env(envName).ToInt(456))

	os.Setenv(envName, "abc")
	assert.Equal(t, 456, Env(envName).ToInt(456))

	os.Setenv(envName, "123a")
	assert.Equal(t, 456, Env(envName).ToInt(456))
}
