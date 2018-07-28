// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"runtime"
	"runtime/debug"
)

// ProcessMeta defines the process statistics collected for reporting
type ProcessMeta struct {
	// the current number of goroutine
	numGoroutine int

	// the memory statistics
	mem *runtime.MemStats

	// the garbage collection statistics
	gc *debug.GCStats
}
