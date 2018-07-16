// Copyright (C) 2017 Librato, Inc. All rights reserved.

package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConfig(t *testing.T) {
	agentConf.initialized = false
	assert.Equal(t, "", GetConfig(ConfName("APPOPTICS_COLLECTOR")))

	agentConf.initialized = true
	assert.Equal(t, "", GetConfig(ConfName("INVALID")))

	assert.Equal(t, "collector.appoptics.com:443", GetConfig(ConfName("APPOPTICS_COLLECTOR")))
	assert.Equal(t, "", GetConfig(ConfName("")))

	os.Setenv("APPOPTICS_COLLECTOR", "test.com:12345")
	Init()
	assert.Equal(t, "test.com:12345", GetConfig(ConfName("APPOPTICS_COLLECTOR")))
	os.Unsetenv("APPOPTICS_COLLECTOR")
	Init()
	assert.Equal(t, "collector.appoptics.com:443", GetConfig(ConfName("APPOPTICS_COLLECTOR")))
}

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
