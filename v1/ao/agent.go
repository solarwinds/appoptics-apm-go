// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"context"
	"io"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/pkg/errors"
)

var (
	errInvalidLogLevel = errors.New("invalid log level")
)

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

func flushAgent() error {
	return reporter.Flush()
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
	l, ok := log.ToLogLevel(level)
	if !ok {
		return errInvalidLogLevel
	}
	log.SetLevel(l)
	return nil
}

// GetLogLevel returns the current logging level of the AppOptics agent
func GetLogLevel() string {
	return log.LevelStr[log.Level()]
}

// SetLogOutput sets the output destination for the internal logger.
func SetLogOutput(w io.Writer) {
	log.SetOutput(w)
}

// SetServiceKey sets the service key of the agent
func SetServiceKey(key string) {
	reporter.SetServiceKey(key)
}
