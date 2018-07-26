// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidServiceKey(t *testing.T) {

	keyPairs := map[string]bool{
		"ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go": true,
		"":       false,
		"abc:Go": false,
		"ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:" +
			"Go0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" +
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" +
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" +
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef": false,
		"1234567890abcdef":  false,
		"1234567890abcdef:": false,
		":Go":               false,
		"abc:123:Go":        false,
	}

	for key, valid := range keyPairs {
		assert.Equal(t, valid, IsValidServiceKey(key))
	}
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

func TestIsValidTracingMode(t *testing.T) {
	assert.Equal(t, true, IsValidTracingMode("always"))
	assert.Equal(t, true, IsValidTracingMode("never"))
	assert.Equal(t, false, IsValidTracingMode("abc"))
	assert.Equal(t, false, IsValidTracingMode(""))
	assert.Equal(t, true, IsValidTracingMode("ALWAYS"))
}
