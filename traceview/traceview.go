package traceview

import (
	"os"
	"strconv"
	"strings"
	"unsafe"
)

/*
#cgo pkg-config: openssl
#cgo LDFLAGS: -loboe
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"

var udp_reporter *Reporter
var settings Settings
var emptyCString *C.char
var layerCache *CStringCache

// Global configuration settings (sample rate, tracing mode.)
type Settings struct {
	settings_cfg C.oboe_settings_cfg_t
}

// Initialize Traceview C instrumentation library ("oboe"):
func init() {
	C.oboe_init()
	udp_reporter = NewUDPReporter()

	// Configure sample rate and tracing mode settings using environment variables:
	C.oboe_settings_cfg_init(&settings.settings_cfg)
	sample_rate, err := strconv.Atoi(os.Getenv("GO_TRACEVIEW_SAMPLE_RATE"))
	if err == nil {
		settings.settings_cfg.sample_rate = C.int(sample_rate)
	}

	mode := strings.ToLower(os.Getenv("GO_TRACEVIEW_TRACING_MODE"))
	switch mode {
	case "always":
	default:
		settings.settings_cfg.tracing_mode = C.OBOE_TRACE_ALWAYS
	case "through":
		settings.settings_cfg.tracing_mode = C.OBOE_TRACE_THROUGH
	case "never":
		settings.settings_cfg.tracing_mode = C.OBOE_TRACE_NEVER
	}

	// To save a malloc/free, we preallocate an empty C string that can be passed into C
	emptyCString = C.CString("")
	layerCache = NewCStringCache()
}

// Determines if request should be traced, based on sample rate settings:
// This is our only dependency on the liboboe C library.
func ShouldTraceRequest(layer, xtrace_header string) (bool, int, int) {
	var sample_rate, sample_source C.int
	var clayer *C.char = layerCache.Get(layer)
	var cxt *C.char

	if xtrace_header == "" {
		// common case, where we are the entry layer
		cxt = emptyCString
	} else {
		cxt = C.CString(xtrace_header)
	}

	sample := int(C.oboe_sample_request(clayer, cxt, &settings.settings_cfg, &sample_rate, &sample_source))

	if cxt != emptyCString {
		C.free(unsafe.Pointer(cxt))
	}

	return sample != 0, int(sample_rate), int(sample_source)
}
