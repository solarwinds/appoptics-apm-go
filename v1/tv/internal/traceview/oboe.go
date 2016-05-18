// +build traceview

// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

/*
#cgo LDFLAGS: -loboe
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"
import (
	"os"
	"runtime"
	"strings"
	"sync"
)

var settings Settings
var emptyCString, inXTraceCString *C.char
var oboeVersion string
var layerCache *CStringCache
var initMessageSent = false

// Global configuration settings (sample rate, tracing mode.)
type Settings struct {
	settings_cfg C.oboe_settings_cfg_t
}

// Initialize Traceview C instrumentation library ("oboe"):
func init() {
	C.oboe_init()
	reporter = NewReporter()
	readEnvSettings()

	// To save on malloc/free, preallocate empty & "non-empty" CStrings
	emptyCString = C.CString("")
	inXTraceCString = C.CString("in_xtrace")
	oboeVersion = C.GoString(C.oboe_config_get_version_string())
	layerCache = NewCStringCache()
}

func readEnvSettings() {
	// Configure tracing mode setting using environment variable
	C.oboe_settings_cfg_init(&settings.settings_cfg)
	mode := strings.ToLower(os.Getenv("GO_TRACEVIEW_TRACING_MODE"))
	switch mode {
	case "always":
		fallthrough
	default:
		settings.settings_cfg.tracing_mode = C.OBOE_TRACE_ALWAYS
	case "through":
		settings.settings_cfg.tracing_mode = C.OBOE_TRACE_THROUGH
	case "never":
		settings.settings_cfg.tracing_mode = C.OBOE_TRACE_NEVER
	}
}

var initVersion = 1
var initLayer = "go"
var initOnce sync.Once

func sendInitMessage() {
	ctx := NewContext()
	ctx.reportEvent(LabelEntry, initLayer, false,
		"__Init", 1,
		"Go.Version", runtime.Version(),
		"Go.Oboe.Version", initVersion,
		"Oboe.Version", oboeVersion,
	)
	ctx.ReportEvent(LabelExit, initLayer)
}

func oboeSampleRequest(layer, xtrace_header string) (bool, int, int) {
	if usingTestReporter {
		if r, ok := reporter.(*testReporter); ok {
			return r.ShouldTrace, 1000000, 2 // trace tests
		}
	}
	initOnce.Do(sendInitMessage)

	var sample_rate, sample_source C.int
	var clayer *C.char = layerCache.Get(layer)
	var cxt *C.char
	if xtrace_header == "" {
		// common case, where we are the entry layer
		cxt = emptyCString
	} else {
		// use const "in_xtrace" arg, oboe_sample_request only checks if it is non-empty
		cxt = inXTraceCString
	}

	sample := int(C.oboe_sample_request(clayer, cxt, &settings.settings_cfg, &sample_rate, &sample_source))

	return sample != 0, int(sample_rate), int(sample_source)
}
