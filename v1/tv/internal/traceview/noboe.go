// +build !traceview

// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

func oboeSampleRequest(layer, xtrace_header string) (bool, int, int) {
	if r, ok := reporter.(*testReporter); ok {
		return r.ShouldTrace, 1000000, 2 // trace tests
	} else {
		return false, 0, 6
	}
}
