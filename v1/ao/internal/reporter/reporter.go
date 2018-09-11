// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"strings"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/host"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
)

// defines what methods a reporter should offer (internal to reporter package)
type reporter interface {
	// called when an event should be reported
	reportEvent(ctx *oboeContext, e *event) error
	// called when a status (e.g. __Init message) should be reported
	reportStatus(ctx *oboeContext, e *event) error
	// called when a Span message should be reported
	reportSpan(span SpanMessage) error
	// Shutdown closes the reporter.
	Shutdown(duration time.Duration) error
	// Closed returns if the reporter is already closed.
	Closed() bool
	// IsReady checks the state of the reporter and may wait for up to the specified
	// duration until it becomes ready.
	IsReady(duration time.Duration) bool
}

// currently used reporter
var globalReporter reporter = &nullReporter{}

var (
	reportingDisabled     = false // reporting disabled due to error
	periodicTasksDisabled = false // disable periodic tasks, for testing
)

// a noop reporter
type nullReporter struct{}

func (r *nullReporter) reportEvent(ctx *oboeContext, e *event) error  { return nil }
func (r *nullReporter) reportStatus(ctx *oboeContext, e *event) error { return nil }
func (r *nullReporter) reportSpan(span SpanMessage) error             { return nil }
func (r *nullReporter) Shutdown(duration time.Duration) error         { return nil }
func (r *nullReporter) Closed() bool                                  { return true }
func (r *nullReporter) IsReady(duration time.Duration) bool           { return true }

// init() is called only once on program startup. Here we create the reporter
// that will be used throughout the runtime of the app. Default is 'ssl' but
// can be overridden via APPOPTICS_REPORTER
func init() {
	checkHostname(os.Hostname)
	setGlobalReporter(config.GetReporterType())
	sendInitMessage()
}

func setGlobalReporter(reporterType string) {
	// Close the previous reporter
	if globalReporter != nil {
		globalReporter.Shutdown(0)
	}

	switch strings.ToLower(reporterType) {
	case "ssl":
		fallthrough // using fallthrough since the SSL reporter (GRPC) is our default reporter
	default:
		globalReporter = newGRPCReporter()
	case "udp":
		globalReporter = udpNewReporter()
	case "none":
	}
}

// IsReady checks the state of the reporter and may wait for up to the specified
// duration until it becomes ready.
func IsReady(tm time.Duration) bool {
	// globalReporter is not protected by a mutex as currently it's only modified
	// from the init() function.
	return globalReporter.IsReady(tm)
}

// ReportSpan is called from the app when a span message is available
// span	span message to be put on the channel
//
// returns	error if channel is full
func ReportSpan(span SpanMessage) error {
	return globalReporter.reportSpan(span)
}

func checkHostname(getter func() (string, error)) {
	if _, err := getter(); err != nil {
		log.Error("Unable to get hostname, AppOptics tracing disabled.")
		reportingDisabled = true
	}
}

// check if context and event are valid, add general keys like Timestamp, or hostname
// ctx		oboe context
// e		event to be prepared for sending
//
// returns	error if invalid context or event
func prepareEvent(ctx *oboeContext, e *event) error {
	if ctx == nil || e == nil {
		return errors.New("invalid context, event")
	}

	// The context metadata must have the same task_id as the event.
	if !bytes.Equal(ctx.metadata.ids.taskID, e.metadata.ids.taskID) {
		return errors.New("invalid event, different task_id from context")
	}

	// The context metadata must have a different op_id than the event.
	if bytes.Equal(ctx.metadata.ids.opID, e.metadata.ids.opID) {
		return errors.New("invalid event, same as context")
	}

	us := time.Now().UnixNano() / 1000
	e.AddInt64("Timestamp_u", us)

	e.AddString("Hostname", host.Hostname())
	e.AddInt("PID", host.PID())

	// Update the context's op_id to that of the event
	ctx.metadata.ids.setOpID(e.metadata.ids.opID)

	bsonBufferFinish(&e.bbuf)
	return nil
}

// Determines if request should be traced, based on sample rate settings.
func shouldTraceRequest(layer string, traced bool) (bool, int, sampleSource) {
	return oboeSampleRequest(layer, traced)
}

func argsToMap(capacity, ratePerSec float64, metricsFlushInterval, maxTransactions int) *map[string][]byte {
	args := make(map[string][]byte)

	if capacity > -1 {
		bits := math.Float64bits(capacity)
		bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, bits)
		args["BucketCapacity"] = bytes
	}
	if ratePerSec > -1 {
		bits := math.Float64bits(ratePerSec)
		bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, bits)
		args["BucketRate"] = bytes
	}
	if metricsFlushInterval > -1 {
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(metricsFlushInterval))
		args["MetricsFlushInterval"] = bytes
	}
	if maxTransactions > -1 {
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, uint32(maxTransactions))
		args["MaxTransactions"] = bytes
	}

	return &args
}
