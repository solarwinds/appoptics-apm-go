// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

// defines what methods a reporter should offer (internal to traceview package)
type reporter interface {
	// called when an event should be reported
	reportEvent(ctx *oboeContext, e *event) error
	// called when a status (e.g. __Init message) should be reported
	reportStatus(ctx *oboeContext, e *event) error
	// called when a Span message should be reported
	reportSpan(span *SpanMessage) error
}

// currently used reporter
var thisReporter reporter = &nullReporter{}

var (
	reportingDisabled     = false // reporting disabled due to error
	periodicTasksDisabled = false // disable periodic tasks, for testing
)

// cached hostname and PID since they don't change (only one system call each)
var cachedHostname string
var cachedPid = os.Getpid()

// a noop reporter
type nullReporter struct{}

func (r *nullReporter) reportEvent(ctx *oboeContext, e *event) error  { return nil }
func (r *nullReporter) reportStatus(ctx *oboeContext, e *event) error { return nil }
func (r *nullReporter) reportSpan(span *SpanMessage) error            { return nil }

// init() is called only once on program startup. Here we create the reporter
// that will be used throughout the runtime of the app. Default is 'ssl' but
// can be overridden via APPOPTICS_REPORTER
func init() {
	cacheHostname(osHostnamer{})

	switch strings.ToLower(os.Getenv("APPOPTICS_REPORTER")) {
	case "ssl":
		fallthrough // using fallthrough since the SSL reporter (GRPC) is our default reporter
	default:
		thisReporter = grpcNewReporter()
	case "udp":
		thisReporter = udpNewReporter()
	}

	sendInitMessage()
}

// called from the app when a span message is available
// span	span message to be put on the channel
//
// returns	error if channel is full
func ReportSpan(span SpanMessage) error {
	return thisReporter.reportSpan(&span)
}

// cache hostname
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

// check if context and event are valid, add general keys like Timestamp, or Hostname
// ctx		oboe context
// e		event to be prepared for sending
//
// returns	error if invalid context or event
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
func shouldTraceRequest(layer string, traced bool) (bool, int, sampleSource) {
	return oboeSampleRequest(layer, traced)
}
