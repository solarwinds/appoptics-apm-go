// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"context"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	aolog "github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/pkg/errors"
)

var (
	errInvalidLogLevel = errors.New("invalid log level")
)

// The `const` variable which should not be updated on runtime.
// These variables are accessed by every request in the critical path, which
// makes it too expensive to be protected by a mutex.
//
// Do NOT modify it outside the init() function.
var (
	// This flag indicates whether the agent is disabled.
	//
	// It is initialized when the package is imported and won't be changed
	// in runtime.
	disabled = false

	// This flag indicates whether the domain name needs to be prepended to
	// the transaction name of the span message
	trxNamePrependDomain = false
)

func init() {
	initDisabled()
	initTrxNamePrependDomain()
}

func initDisabled() {
	disabled = config.GetDisabled()
	if disabled {
		aolog.Warningf("AppOptics agent is disabled.")
	}
}

func initTrxNamePrependDomain() {
	trxNamePrependDomain = config.GetPrependDomain()
}

// Disabled indicates if the agent is disabled
func Disabled() bool {
	return disabled
}

// WaitForReady checks if the agent is ready. It returns true is the agent is ready,
// or false if it is not.
//
// A call to this method will block until the agent is ready or the context is
// canceled, or the agent is already closed.
// The agent is considered ready if there is a valid default setting for sampling.
func WaitForReady(ctx context.Context) bool {
	if Closed() {
		return false
	}
	return reporter.WaitForReady(ctx)
}

// Shutdown flush the metrics and stops the agent. The call will block until the agent
// flushes and is successfully shutdown or the context is canceled. It returns nil
// for successful shutdown and or error when the context is canceled or the agent
// has already been closed before.
//
// This function should be called only once.
func Shutdown(ctx context.Context) error {
	return reporter.Shutdown(ctx)
}

// Closed denotes if the agent is closed (by either calling Shutdown explicitly
// or being triggered from some internal error).
func Closed() bool {
	return reporter.Closed()
}

// SetLogLevel changes the logging level of the AppOptics agent
// Valid logging levels: DEBUG, INFO, WARN, ERROR
func SetLogLevel(level string) error {
	l, ok := aolog.ToLogLevel(level)
	if !ok {
		return errInvalidLogLevel
	}
	aolog.SetLevel(l)
	return nil
}

// GetLogLevel returns the current logging level of the AppOptics agent
func GetLogLevel() string {
	return aolog.LevelStr[aolog.Level()]
}
