// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/pkg/errors"
)

// EnvVar defines the necessary properties and behaviors of a environment variable
type EnvVar struct {
	// the name of the environment variable
	name string
}

// Env creates a new EnvVar object
func Env(name string) EnvVar {
	return EnvVar{name}
}

// ToString loads the env and returns a string value
func (e EnvVar) ToString(fallback string) string {
	v := os.Getenv(e.name)
	if v != "" {
		return v
	}
	return fallback
}

func toBool(s string) (bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "yes" || s == "true" {
		return true, nil
	} else if s == "no" || s == "false" {
		return false, nil
	}
	return false, errors.New("cannot convert input to bool")
}

// ToBool loads the env and returns a bool value
func (e EnvVar) ToBool(fallback bool) bool {
	v := os.Getenv(e.name)
	if v == "" {
		return fallback
	}
	if b, err := toBool(v); err != nil {
		log.Warningf("Ignore invalid bool value: %s=%s", e.name, v)
		return fallback
	} else {
		return b
	}
}

// ToInt loads the env and returns a int value
func (e EnvVar) ToInt(fallback int) int {
	return int(e.ToInt64(int64(fallback)))
}

// ToInt64 loads the env and returns a int64 value
func (e EnvVar) ToInt64(fallback int64) int64 {
	v := os.Getenv(e.name)
	if v == "" {
		return fallback
	}

	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Warningf("Ignore invalid int/int64 value: %s=%s", e.name, v)
		return fallback
	}
	return i
}
