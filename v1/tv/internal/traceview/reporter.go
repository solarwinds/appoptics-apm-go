package traceview

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

type Reporter interface {
	ReportEvent(ctx *oboeContext, e *event) error
	ReportSpan(span *HttpSpanMessage) error
}

type nullReporter struct{}

var reporter Reporter = &nullReporter{}

var reportingDisabled bool = false

var cachedHostname string
var cachedPid = os.Getpid()

func (r *nullReporter) ReportEvent(ctx *oboeContext, e *event) error { return nil }
func (r *nullReporter) ReportSpan(span *HttpSpanMessage) error       { return nil }

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

func prepareEvent(ctx *oboeContext, e *event) error {
	if ctx == nil || e == nil {
		return errors.New("Invalid context, event")
	}

	// The context metadata must have the same task_id as the event.
	if !bytes.Equal(ctx.metadata.ids.taskID, e.metadata.ids.taskID) {
		return errors.New("Invalid event, different task_id from context")
	}

	// The context metadata must have a different op_id than the event.
	if bytes.Equal(ctx.metadata.ids.opID, e.metadata.ids.opID) {
		return errors.New("Invalid event, same as context")
	}

	us := time.Now().UnixNano() / 1000
	e.AddInt64("Timestamp_u", us)

	// Add cached syscalls for Hostname & PID
	e.AddString("Hostname", cachedHostname)
	e.AddInt("PID", cachedPid)

	// Update the context's op_id to that of the event
	ctx.metadata.ids.setOpID(e.metadata.ids.opID)

	bsonBufferFinish(&e.bbuf)
	return nil
}

// Determines if request should be traced, based on sample rate settings:
// This is our only dependency on the liboboe C library.
func shouldTraceRequest(layer, xtraceHeader string) (sampled bool, sampleRate, sampleSource int) {
	return oboeSampleRequest(layer, xtraceHeader)
}
