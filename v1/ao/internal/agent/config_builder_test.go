// Copyright (C) 2017 Librato, Inc. All rights reserved.

package agent

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitConf(t *testing.T) {
	var buffer bytes.Buffer

	r := os.Getenv("APPOPTICS_REPORTER")
	os.Unsetenv("APPOPTICS_REPORTER")
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	os.Setenv("APPOPTICS_SERVICE_KEY", "1234567890abcdef:go")
	log.SetOutput(&buffer)
	Init()
	assert.True(t, strings.HasSuffix(buffer.String(), "non-default configuration used APPOPTICS_DEBUG_LEVEL=debug\n"))

	os.Setenv("APPOPTICS_REPORTER", r)
}

func TestMaskServiceKey(t *testing.T) {
	keyPairs := map[string]string{
		"1234567890abcdef:Go": "1234********cdef:Go",
		"abc:Go":              "abc:Go",
		"abcd1234:Go":         "abcd1234:Go",
	}

	for key, masked := range keyPairs {
		assert.Equal(t, masked, maskServiceKey(key))
	}
}
