// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// TestReporter appends reported events to Bufs if ShouldTrace is true.
type TestReporter struct {
	EventBufs             [][]byte
	SpanMessages          []SpanMessage
	ShouldTrace           bool
	ShouldError           bool
	UseSettings           bool
	DisableDefaultSetting bool
	CaptureMetrics        bool
	ErrorEvents           map[int]bool // whether to drop an event
	eventCount            int64
	done                  chan int
	wg                    sync.WaitGroup
	eventChan             chan []byte
	spanMsgChan           chan SpanMessage
	Timeout               time.Duration
}

const defaultTestReporterTimeout = 2 * time.Second

var usingTestReporter = false
var oldReporter reporter = &nullReporter{}

// TestReporterOption values may be passed to SetTestReporter.
type TestReporterOption func(*TestReporter)

// TestReporterDisableDefaultSetting disables the default 100% test sampling rate and leaves settings state empty.
func TestReporterDisableDefaultSetting(val bool) TestReporterOption {
	return func(r *TestReporter) { r.DisableDefaultSetting = val }
}

// TestReporterTimeout sets a timeout for the TestReporter to wait before shutting down its writer.
func TestReporterTimeout(timeout time.Duration) TestReporterOption {
	return func(r *TestReporter) { r.Timeout = timeout }
}

// TestReporterDisableTracing turns off settings lookup and ensures oboeSampleRequest returns false.
func TestReporterDisableTracing() TestReporterOption {
	return func(r *TestReporter) {
		r.DisableDefaultSetting = true
		r.ShouldTrace = false
		r.UseSettings = false
	}
}

// TestReporterShouldTrace sets the first argument of the return value of oboeSampleRequest().
func TestReporterShouldTrace(val bool) TestReporterOption {
	return func(r *TestReporter) {
		r.ShouldTrace = val
	}
}

// TestReporterUseSettings sets whether to look up settings lookup or return the value of r.ShouldTrace.
func TestReporterUseSettings(val bool) TestReporterOption {
	return func(r *TestReporter) { r.UseSettings = val }
}

// SetTestReporter sets and returns a test reporter that captures raw event bytes
// for making assertions about using the graphtest package.
func SetTestReporter(options ...TestReporterOption) *TestReporter {
	r := &TestReporter{
		ShouldTrace: true,
		UseSettings: true,
		Timeout:     defaultTestReporterTimeout,
		done:        make(chan int),
		eventChan:   make(chan []byte),
		spanMsgChan: make(chan SpanMessage),
	}
	for _, option := range options {
		option(r)
	}
	r.wg.Add(1)
	go r.resultWriter()

	if _, ok := oldReporter.(*nullReporter); ok {
		oldReporter = globalReporter
	}
	globalReporter = r
	usingTestReporter = true

	// start with clean slate
	resetSettings()

	// set default setting with 100% sampling rate
	if !r.DisableDefaultSetting {
		r.addDefaultSetting()
	}

	return r
}

func (r *TestReporter) resultWriter() {
	var numBufs int
	for {
		select {
		case numBufs = <-r.done:
			if len(r.EventBufs)+len(r.SpanMessages) >= numBufs {
				r.wg.Done()
				return
			}
			r.done = nil
		case <-time.After(r.Timeout):
			r.wg.Done()
			return
		case buf := <-r.eventChan:
			r.EventBufs = append(r.EventBufs, buf)
			if r.done == nil && len(r.EventBufs)+len(r.SpanMessages) >= numBufs {
				r.wg.Done()
				return
			}
		case buf := <-r.spanMsgChan:
			r.SpanMessages = append(r.SpanMessages, buf)
			if r.done == nil && len(r.EventBufs)+len(r.SpanMessages) >= numBufs {
				r.wg.Done()
				return
			}
		}
	}
}

// Close stops the test reporter from listening for events; r.EventBufs will no longer be updated and any
// calls to WritePacket() will panic.
func (r *TestReporter) Close(numBufs int) {
	r.done <- numBufs
	// wait for reader goroutine to receive numBufs events, or timeout.
	r.wg.Wait()
	close(r.eventChan)
	received := len(r.EventBufs) + len(r.SpanMessages)
	if received < numBufs {
		log.Printf("# FIX: TestReporter.Close() waited for %d events, got %d", numBufs, received)
	}
	usingTestReporter = false
	if _, ok := oldReporter.(*nullReporter); !ok {
		globalReporter = oldReporter
		oldReporter = &nullReporter{}
	}
}

// Shutdown closes the Test reporter TODO: not supported
func (r *TestReporter) Shutdown() error {
	// return r.conn.Close()
	return errors.New("shutdown is not supported by TestReporter")
}

// Closed returns if the reporter is closed or not TODO: not supported
func (r *TestReporter) Closed() bool {
	return false
}

func (r *TestReporter) report(ctx *oboeContext, e *event) error {
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	atomic.AddInt64(&r.eventCount, 1)
	if r.ShouldError || // error all events
		(r.ErrorEvents != nil && r.ErrorEvents[(int(r.eventCount)-1)]) { // error certain specified events
		return errors.New("TestReporter error")
	}
	r.eventChan <- (*e).bbuf.GetBuf() // a send to a closed channel panics.
	return nil
}

func (r *TestReporter) reportEvent(ctx *oboeContext, e *event) error {
	return r.report(ctx, e)
}

func (r *TestReporter) reportStatus(ctx *oboeContext, e *event) error {
	return r.report(ctx, e)
}

func (r *TestReporter) reportSpan(span SpanMessage) error {
	r.spanMsgChan <- span
	return nil
}

func (r *TestReporter) addDefaultSetting() {
	// add default setting with 100% sampling
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
}
