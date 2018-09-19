package utils

import "runtime"

var (
	// The AppOptics Go agent version
	version = "v1.3.0"

	// The Go version
	goVersion = runtime.Version()
)

// Version returns the agent's version
func Version() string {
	return version
}

// GoVersion returns the Go version
func GoVersion() string {
	return goVersion
}
