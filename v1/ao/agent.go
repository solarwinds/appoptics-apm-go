// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"context"

	aolog "github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/pkg/errors"
)

var (
	errInvalidLogLevel = errors.New("invalid log level")
)

// WaitForReady checks if the agent is ready. It will block until the agent is ready
// or the context is canceled.
//
// The agent is considered ready if there is a valid default setting for sampling.
func WaitForReady(ctx context.Context) bool {
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
