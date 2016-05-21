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

var globalSettings settings
var emptyCString, inXTraceCString *C.char
var oboeVersion string
var layerCache *cStringCache

// Global configuration settings (sample rate, tracing mode.)
type settings struct {
	settings_cfg C.oboe_settings_cfg_t
}

// Initialize Traceview C instrumentation library ("oboe"):
func init() {
	C.oboe_init()
	globalReporter = newReporter()
	readEnvSettings()

	// To save on malloc/free, preallocate empty & "non-empty" CStrings
	emptyCString = C.CString("")
	inXTraceCString = C.CString("in_xtrace")
	oboeVersion = C.GoString(C.oboe_config_get_version_string())
	layerCache = newCStringCache()
}

func readEnvSettings() {
	// Configure tracing mode setting using environment variable
	C.oboe_settings_cfg_init(&globalSettings.settings_cfg)
	mode := strings.ToLower(os.Getenv("GO_TRACEVIEW_TRACING_MODE"))
	switch mode {
	case "always":
		fallthrough
	default:
		globalSettings.settings_cfg.tracing_mode = C.OBOE_TRACE_ALWAYS
	case "through":
		globalSettings.settings_cfg.tracing_mode = C.OBOE_TRACE_THROUGH
	case "never":
		globalSettings.settings_cfg.tracing_mode = C.OBOE_TRACE_NEVER
	}
}

var initVersion = 1
var initLayer = "go"
var initMessageOnce sync.Once

func sendInitMessage() {
	ctx := newContext()
	if c, ok := ctx.(*context); ok {
		c.reportEvent(LabelEntry, initLayer, false,
			"__Init", 1,
			"Go.Version", runtime.Version(),
			"Go.Oboe.Version", initVersion,
			"Oboe.Version", oboeVersion,
		)
		c.ReportEvent(LabelExit, initLayer)
	}
}

func oboeSampleRequest(layer, xtrace_header string) (bool, int, int) {
	if usingTestReporter {
		if r, ok := globalReporter.(*TestReporter); ok {
			return r.ShouldTrace, 1000000, 2 // trace tests
		}
	}
	initMessageOnce.Do(sendInitMessage)

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

	sample := int(C.oboe_sample_request(clayer, cxt, &globalSettings.settings_cfg, &sample_rate, &sample_source))

	return sample != 0, int(sample_rate), int(sample_source)
}
