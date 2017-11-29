// +build disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

func oboeSampleRequest(layer string, traced bool) (bool, int, sampleSource) {
	if usingTestReporter {
		if r, ok := thisReporter.(*TestReporter); ok {
			return r.ShouldTrace, 1000000, 2 // trace tests
		}
	}
	return false, 0, 6
}

func updateSetting(sType int32, layer string, flags []byte, value int64, ttl int64, arguments *map[string][]byte) {
}

func resetSettings() {}

func flushRateCounts() *rateCounts { return &rateCounts{} }

func sendInitMessage() {}
