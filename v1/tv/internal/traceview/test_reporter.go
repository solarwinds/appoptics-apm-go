// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// SetTestReporter sets and returns a test reporter that captures raw event bytes
// for making assertions about using the graphtest package.
func SetTestReporter(args ...interface{}) *TestReporter {
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
	globalReporter = r
	usingTestReporter = true
	go r.resultWriter()
	return r
}

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
}

// WritePacket appends buf to Bufs; if TestReporter.Close() was called it will panic.
func (r *TestReporter) WritePacket(buf []byte) (int, error) {
	atomic.AddInt64(&r.eventCount, 1)
	if r.ShouldError || // error all events
		(r.ErrorEvents != nil && r.ErrorEvents[(int(r.eventCount)-1)]) { // error certain specified events
		return 0, errors.New("TestReporter error")
	}
	r.bufChan <- buf // a send to a closed channel panics.
	return len(buf), nil
}

// IsOpen is always true.
func (r *TestReporter) IsOpen() bool { return true }

func (r *TestReporter) IsMetricsConnOpen() bool { return true}

// PushMetricsRecord is invoked by a trace to push the mAgg record
func (r *TestReporter) PushMetricsRecord(record MetricsRecord) bool { return true } // TODO: process metrics record
