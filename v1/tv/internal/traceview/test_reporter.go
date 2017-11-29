// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// TestReporter appends reported events to Bufs if ShouldTrace is true.
type TestReporter struct {
	Bufs        [][]byte
	ShouldTrace bool
	ShouldError bool
	ErrorEvents map[int]bool // whether to drop an event
	eventCount  int64
	done        chan int
	wg          sync.WaitGroup
	bufChan     chan []byte
	Timeout     time.Duration
}

var usingTestReporter = false
var oldReporter reporter = &nullReporter{}

// SetTestReporter sets and returns a test reporter that captures raw event bytes
// for making assertions about using the graphtest package.
func SetTestReporter(withDefaultSetting bool, args ...interface{}) *TestReporter {
	timeout := 2 * time.Second
	if len(args) == 1 {
		timeout, _ = args[0].(time.Duration)
	}
	r := &TestReporter{
		ShouldTrace: true,
		Timeout:     timeout,
		done:        make(chan int),
		bufChan:     make(chan []byte),
	}
	go r.resultWriter()

	if _, ok := oldReporter.(*nullReporter); ok {
		oldReporter = thisReporter
	}
	thisReporter = r
	usingTestReporter = true

	// start with clean slate
	resetSettings()

	// set default setting with 100% sampling rate
	if withDefaultSetting {
		r.addDefaultSetting()
	}

	return r
}

func (r *TestReporter) resultWriter() {
	r.wg.Add(1)
	var numBufs int
	for {
		select {
		case numBufs = <-r.done:
			if len(r.Bufs) == numBufs {
				r.wg.Done()
				return
			}
			r.done = nil
		case <-time.After(r.Timeout):
			r.wg.Done()
			return
		case buf := <-r.bufChan:
			r.Bufs = append(r.Bufs, buf)
			if r.done == nil && len(r.Bufs) == numBufs {
				r.wg.Done()
				return
			}
		}
	}
}

// Close stops the test reporter from listening for events; r.Bufs will no longer be updated and any
// calls to WritePacket() will panic.
func (r *TestReporter) Close(numBufs int) {
	r.done <- numBufs
	// wait for reader goroutine to receive numBufs events, or timeout.
	r.wg.Wait()
	close(r.bufChan)

	usingTestReporter = false
	if _, ok := oldReporter.(*nullReporter); !ok {
		thisReporter = oldReporter
		oldReporter = &nullReporter{}
	}
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
	r.bufChan <- (*e).bbuf.GetBuf() // a send to a closed channel panics.
	return nil
}

func (r *TestReporter) reportEvent(ctx *oboeContext, e *event) error {
	return r.report(ctx, e)
}

func (r *TestReporter) reportStatus(ctx *oboeContext, e *event) error {
	return r.report(ctx, e)
}

func (r *TestReporter) reportSpan(span *SpanMessage) error {
	s := (*span).(*HttpSpanMessage)
	bbuf := NewBsonBuffer()
	bsonAppendString(bbuf, "transaction", s.Transaction)
	bsonAppendString(bbuf, "url", s.Url)
	bsonAppendInt(bbuf, "status", s.Status)
	bsonAppendString(bbuf, "method", s.Method)
	bsonAppendBool(bbuf, "hasError", s.HasError)
	bsonAppendInt64(bbuf, "duration", s.Duration.Nanoseconds())
	bsonBufferFinish(bbuf)
	r.bufChan <- bbuf.buf
	return nil
}

func (r *TestReporter) addDefaultSetting() {
	// add default setting with 100% sampling
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(1000000, 1000000, -1, -1))
}
