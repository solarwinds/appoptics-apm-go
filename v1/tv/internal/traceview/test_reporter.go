// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/librato/go-traceview/v1/tv/internal/traceview/collector"
	"google.golang.org/grpc"
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
	go r.resultWriter()
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
}

func (r *TestReporter) reportEvent(ctx *oboeContext, e *event) error {
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

func (r *TestReporter) reportSpan(span *SpanMessage) error { return nil }

type testCollectorClient struct {
	mReq chan interface{}
	mRes interface{}
	err  error
}

func (t *testCollectorClient) GetReq() interface{} {
	return t.mReq
}

func (t *testCollectorClient) SetRes(res interface{}, err error) {
	t.mRes = res
	t.err = err
}

func (t *testCollectorClient) PostEvents(ctx context.Context, in *collector.MessageRequest,
	opts ...grpc.CallOption) (*collector.MessageResult, error) {
	t.mReq <- in
	return t.mRes.(*collector.MessageResult), t.err
}

func (t *testCollectorClient) PostMetrics(ctx context.Context, in *collector.MessageRequest,
	opts ...grpc.CallOption) (*collector.MessageResult, error) {
	t.mReq <- in
	return t.mRes.(*collector.MessageResult), t.err
}

func (t *testCollectorClient) PostStatus(ctx context.Context, in *collector.MessageRequest,
	opts ...grpc.CallOption) (*collector.MessageResult, error) {
	t.mReq <- in
	return t.mRes.(*collector.MessageResult), t.err
}

func (t *testCollectorClient) GetSettings(ctx context.Context, in *collector.SettingsRequest,
	opts ...grpc.CallOption) (*collector.SettingsResult, error) {
	t.mReq <- in
	return t.mRes.(*collector.SettingsResult), t.err
}

func (t *testCollectorClient) Ping(ctx context.Context, in *collector.PingRequest,
	opts ...grpc.CallOption) (*collector.MessageResult, error) {
	t.mReq <- in
	return t.mRes.(*collector.MessageResult), t.err
}

func newTestCollectorClient(mRes interface{}, err error) collector.TraceCollectorClient {
	return &testCollectorClient{
		mReq: make(chan interface{}),
		mRes: mRes,
		err:  err,
	}
}
