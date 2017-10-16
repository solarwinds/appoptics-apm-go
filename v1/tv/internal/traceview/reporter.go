package traceview

import (
	"log"
	"os"
	"strings"
)

type Reporter interface {
}

type nullReporter struct{}

var reporter Reporter = &nullReporter{}

var reportingDisabled bool = false

var cachedHostname string
var cachedPid = os.Getpid()

func init() {
	cacheHostname(osHostnamer{})

	switch strings.ToLower(os.Getenv("APPOPTICS_REPORTER")) {
	case "ssl":
		fallthrough
	default:
		reporter = grpcNewReporter()
	case "udp":
		//TODO
	}
}

func cacheHostname(hn hostnamer) {
	h, err := hn.Hostname()
	if err != nil {
		if debugLog {
			log.Printf("Unable to get hostname, AppOptics tracing disabled: %v", err)
		}
		reportingDisabled = true
	}
	cachedHostname = h
}

func reportEvent(r Reporter, ctx *oboeContext, e *event) error {
	return nil
}

// Determines if request should be traced, based on sample rate settings:
// This is our only dependency on the liboboe C library.
func shouldTraceRequest(layer, xtraceHeader string) (sampled bool, sampleRate, sampleSource int) {
	return oboeSampleRequest(layer, xtraceHeader)
}
