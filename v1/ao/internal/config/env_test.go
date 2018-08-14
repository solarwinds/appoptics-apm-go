// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadString(t *testing.T) {
	var envName = "TEST_STRING"

	cases := []struct {
		setEnv   bool
		e        Env
		val      string
		fallback string
		expected string
	}{
		{true,
			Env{
				envName,
				true,
				func(string) bool { return true },
				func(s string) interface{} { return s },
				nil,
			},
			"hello",
			"fallback",
			"hello",
		},
		{true,
			Env{
				envName,
				true,
				func(string) bool { return true },
				func(s string) interface{} { return s },
				nil,
			},
			"",
			"fallback",
			""},
		{true,
			Env{
				envName,
				false,
				func(string) bool { return false },
				func(s string) interface{} { return s },
				nil,
			},
			"",
			"fallback1",
			"fallback1"},
		{false,
			Env{
				envName,
				true,
				func(string) bool { return true },
				func(s string) interface{} { return s },
				nil,
			},
			"",
			"fallback2",
			"fallback2"},
	}

	for i, c := range cases {
		os.Unsetenv(envName)
		if c.setEnv {
			os.Setenv(envName, c.val)
		}
		assert.Equal(t, c.expected, c.e.LoadString(c.fallback), i)
	}
}

func TestLoadBool(t *testing.T) {
	var envName = "TEST_BOOL"

	cases := []struct {
		setEnv   bool
		e        Env
		val      string
		fallback bool
		expected bool
	}{
		{true,
			Env{
				envName,
				true,
				func(string) bool { return true },
				func(s string) interface{} { return s == "true" },
				nil,
			},
			"false",
			true,
			false,
		},
		{true,
			Env{
				envName,
				true,
				func(string) bool { return false },
				func(s string) interface{} { return s == "true" },
				nil,
			},
			"",
			true,
			true},
		{false,
			Env{
				envName,
				true,
				func(string) bool { return false },
				func(s string) interface{} { return s == "true" },
				nil,
			},
			"abc",
			false,
			false},
	}

	for i, c := range cases {
		if c.setEnv {
			os.Setenv(envName, c.val)
		}
		assert.Equal(t, c.expected, c.e.LoadBool(c.fallback), i)
	}
}

func TestLoadInt(t *testing.T) {
	var envName = "TEST_INT"

	cases := []struct {
		setEnv   bool
		e        Env
		val      string
		fallback int
		expected int
	}{
		{true,
			Env{
				envName,
				true,
				func(string) bool { return true },
				func(s string) interface{} {
					i, _ := strconv.Atoi(s)
					return i
				},
				nil,
			},
			"2",
			1,
			2,
		},
	}

	for i, c := range cases {
		if c.setEnv {
			os.Setenv(envName, c.val)
		}
		assert.Equal(t, c.expected, c.e.LoadInt(c.fallback), i)
	}
}
