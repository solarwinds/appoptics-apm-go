// +build disable_tracing

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package reporter

import "sync"

var hasSetting bool = true
var mu sync.Mutex

func OboeCheckSettingsTimeout() {}

func oboeSampleRequest(layer string, traced bool) (bool, int, sampleSource) {
	if usingTestReporter {
		if r, ok := globalReporter.(*TestReporter); ok {
			return r.ShouldTrace, 1000000, 2 // trace tests
		}
	}
	return false, 0, 6
}

func updateSetting(sType int32, layer string, flags []byte, value int64, ttl int64, arguments *map[string][]byte) {
	mu.Lock()
	defer mu.Unlock()
	hasSetting = true
}

func removeSetting(string) {
	mu.Lock()
	defer mu.Unlock()
	hasSetting = false
}

func resetSettings() {}

func flushRateCounts() *rateCounts { return &rateCounts{} }

func sendInitMessage() {}

func hasDefaultSetting() bool {
	mu.Lock()
	defer mu.Unlock()
	ret := hasSetting
	return ret
}