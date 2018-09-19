//go:generate go run codegen.go

package utils

var (
	// The AppOptics Go agent version
	version = "uninitialized"
	// The git commit ID
	commit = "uninitialized"
	// The Go version
	goVersion = "uninitialized"
)

// Version returns the agent's version
func Version() string {
	return version
}

// Commit returns the agent's commit ID
func Commit() string {
	return commit
}

// GoVersion returns the Go version
func GoVersion() string {
	return goVersion
}
