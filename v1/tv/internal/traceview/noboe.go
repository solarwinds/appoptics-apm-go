// +build disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

func oboeSampleRequest(layer, xtraceHeader string) (bool, int, int) {
	if usingTestReporter {
		if r, ok := thisReporter.(*TestReporter); ok {
			return r.ShouldTrace, 1000000, 2 // trace tests
		}
	}
	return false, 0, 6
}
