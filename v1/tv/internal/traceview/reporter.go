// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"log"
	"net"
	"os"
	"time"
)

type Reporter interface {
	WritePacket([]byte) (int, error)
	IsOpen() bool
}

func NewReporter() Reporter {
	var conn *net.UDPConn
	serverAddr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:7831")
	if err == nil {
		conn, err = net.DialUDP("udp4", nil, serverAddr)
	}
	if err != nil {
		log.Printf("Failed to initialize UDP reporter: %v", err)
		return &nullReporter{}
	}
	return &udpReporter{conn: conn}
}

type nullReporter struct{}

func (r *nullReporter) IsOpen() bool                        { return false }
func (r *nullReporter) WritePacket(buf []byte) (int, error) { return len(buf), nil }

type udpReporter struct {
	conn *net.UDPConn
}

func (r *udpReporter) IsOpen() bool                        { return r.conn != nil }
func (r *udpReporter) WritePacket(buf []byte) (int, error) { return r.conn.Write(buf) }

var reporter Reporter = &nullReporter{}
var usingTestReporter bool
var cachedHostname = func() string {
	h, err := os.Hostname()
	if err != nil {
		// TODO report _Error event? use stderr?
		log.Printf("Unable to get hostname, TraceView tracing disabled: %v", err)
		reporter = &nullReporter{} // disable reporting
	}
	return h
}()

var cachedPid = os.Getpid()

func reportEvent(r Reporter, ctx *Context, e *Event) error {
	if !r.IsOpen() {
		// Reporter didn't initialize, nothing to do...
		return nil
	}

	// XXX add validation from oboe_reporter_send

	us := time.Now().UnixNano() / 1000
	e.AddInt64("Timestamp_u", us)

	// Add cached syscalls for Hostname & PID
	e.AddString("Hostname", cachedHostname)
	e.AddInt("PID", cachedPid)

	// Update the context's op_id to that of the event
	oboe_ids_set_op_id(&ctx.metadata.ids, e.metadata.ids.op_id)

	// Send BSON:
	bson_buffer_finish(&e.bbuf)
	_, err := r.WritePacket(e.bbuf.buf)
	return err
}

// Determines if request should be traced, based on sample rate settings:
// This is our only dependency on the liboboe C library.
func shouldTraceRequest(layer, xtraceHeader string) (sampled bool, sampleRate, sampleSource int) {
	return oboeSampleRequest(layer, xtraceHeader)
}

// SetTestReporter sets and returns a test reporter that captures raw event bytes
func SetTestReporter() *testReporter {
	r := &testReporter{ShouldTrace: true}
	reporter = r
	usingTestReporter = true
	return r
}

type testReporter struct {
	Bufs        [][]byte
	ShouldTrace bool
}

func (r *testReporter) WritePacket(buf []byte) (int, error) {
	r.Bufs = append(r.Bufs, buf)
	return len(buf), nil
}

func (r *testReporter) IsOpen() bool { return true }
