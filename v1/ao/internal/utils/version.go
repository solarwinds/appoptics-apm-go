package utils

import (
	"runtime"
	"strings"
)

var (
	// The AppOptics Go agent version
	version = "1.9.0"

	// The Go version
	goVersion = strings.TrimPrefix(runtime.Version(), "go")
)

// Version returns the agent's version
func Version() string {
	return version
}

// GoVersion returns the Go version
func GoVersion() string {
	return goVersion
}
