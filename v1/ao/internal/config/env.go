// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/agent"
)

// Env defines the necessary properties and behaviors of a environment variable
// While loading the environment variable, it validates the value with the
// validator first, if passed it convert the value to a specified data type.
// It also logs significant events during loading, e.g., values that does not
// pass the validation, a mandatory env is missing, or a non-default value.
type Env struct {
	// the name of the environment variable
	name string

	// is the environment variable optional or not
	optional bool

	// the validator function
	validate func(string) bool

	// the converter function
	convert func(string) interface{}

	// the function to mask sensitive data
	mask func(string) string
}

// LoadString loads the env and returns a string value
func (e Env) LoadString(fallback string) string {
	v := e.load(fallback)
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}

// LoadBool loads the env and returns a bool value
func (e Env) LoadBool(fallback bool) bool {
	v := e.load(fallback)
	if s, ok := v.(bool); ok {
		return s
	}
	return fallback
}

// LoadInt loads the env and returns a int value
func (e Env) LoadInt(fallback int) int {
	v := e.load(fallback)
	if s, ok := v.(int); ok {
		return s
	}
	return fallback
}

// LoadInt64 loads the env and returns a int64 value
func (e Env) LoadInt64(fallback int64) int64 {
	v := e.load(fallback)
	if s, ok := v.(int64); ok {
		return s
	}
	return fallback
}

// load loads the environment variable and returns the value
func (e Env) load(fallback interface{}) interface{} {
	validate := e.validate
	if validate == nil {
		validate = func(string) bool { return true }
	}

	convert := e.convert
	if convert == nil {
		convert = func(s string) interface{} { return s }
	}

	if s, ok := os.LookupEnv(e.name); ok {
		if validate(s) {
			e.reportNonDefault(s)
			return convert(s)
		} else {
			e.reportInvalid(s)
			return fallback
		}
	} else {
		if e.optional {
			return fallback
		} else {
			// this is a mandatory variable but missing
			e.reportMissing()
		}
	}
	return fallback
}

func (e Env) reportInvalid(v string) {
	if e.mask != nil {
		v = e.mask(v)
	}
	agent.Warning(InvalidEnv(e.name, v))
}

func (e Env) reportMissing() {
	agent.Warning(MissingEnv(e.name))
}

func (e Env) reportNonDefault(v string) {
	mask := e.mask
	if mask == nil {
		mask = func(s string) string { return s }
	}
	agent.Warning(NonDefaultEnv(e.name, mask(v)))
}
