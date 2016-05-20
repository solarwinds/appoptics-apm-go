// +build !traceview

// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

func oboeSampleRequest(layer, xtraceHeader string) (bool, int, int) {
	if r, ok := globalReporter.(*TestReporter); ok {
		return r.ShouldTrace, 1000000, 2 // trace tests
	}
	return false, 0, 6
}
