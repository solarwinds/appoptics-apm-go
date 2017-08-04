// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"os"
	"time"

	"github.com/librato/go-traceview/v1/tv/internal/traceview/collector"
	"google.golang.org/grpc"
)

// Reporter status
const (
	OK           Status = iota
	DISCONNECTED        // will try to reconnect it later
	RECONNECTING        // reconnecting to gRPC server in progress
	CLOSING             // closed by us, don't do anything to resume it
)

// Reporter maximum retries number
const (
	MaxRetriesNum = ^uint(0) // large enough to mean retry indefinitely
)

// Reporter parameters which are unlikely to change
const (
	maxEventBytes            = 64 * 1024 * 1024
	grpcReporterFlushTimeout = 100 * time.Millisecond
	agentMetricsInterval     = time.Minute
	retryAmplifier           = 30
)

type reporter interface {
	WritePacket([]byte) (int, error)
	IsOpen() bool
	// PushMetricsRecord is invoked by a trace to push the mAgg record
	PushMetricsRecord(record MetricsRecord) bool
}

func newUDPReporter() reporter {
	var conn *net.UDPConn
	if reportingDisabled {
		return &nullReporter{}
	}
	serverAddr, err := net.ResolveUDPAddr("udp4", udpReporterAddr)
	if err == nil {
		conn, err = net.DialUDP("udp4", nil, serverAddr)
	}
	if err != nil {
		if os.Getenv("TRACEVIEW_DEBUG") != "" {
			log.Printf("TraceView failed to initialize UDP reporter: %v", err)
		}
		return &nullReporter{}
	}
	return &udpReporter{conn: conn}
}

type nullReporter struct{}

func (r *nullReporter) IsOpen() bool                                { return false }
func (r *nullReporter) WritePacket(buf []byte) (int, error)         { return len(buf), nil }
func (r *nullReporter) PushMetricsRecord(record MetricsRecord) bool { return true }

type udpReporter struct {
	conn *net.UDPConn
}

func (r *udpReporter) IsOpen() bool                                { return r.conn != nil }
func (r *udpReporter) WritePacket(buf []byte) (int, error)         { return r.conn.Write(buf) }
func (r *udpReporter) PushMetricsRecord(record MetricsRecord) bool { return false }

type Status int

type gRPC struct {
	client        collector.TraceCollectorClient
	status        Status
	retries       uint
	nextRetryTime time.Time
}

type grpcReporter struct {
	client  collector.TraceCollectorClient
	metrics gRPC
	ch      chan []byte
	exit    chan struct{}
	apiKey  string
	mAgg    MetricsAggregator
}

type grpcResult struct {
	result *collector.MessageResult
	err    error
}

func (r *grpcReporter) IsOpen() bool { return r.client != nil } // TODO
func (r *grpcReporter) WritePacket(buf []byte) (int, error) {
	r.ch <- buf
	return len(buf), nil
}

func (r *grpcReporter) reportEvents() {
	// TODO: update reporterCounters in mAgg (numSent, numFailed, etc.) for MetricsMessage
	// TODO: e.g., r.mAgg.IncrementReporterCounter()
	batches := make(chan [][]byte)
	results := r.postEvents(batches)

	var batch [][]byte
	var eventBytes int
	var logIsRunning bool
	flushBatch := func() {
		if !logIsRunning && len(batch) > 0 {
			logIsRunning = true
			batches <- batch
			batch = nil
			eventBytes = 0
		}
	}
	for {
		select {
		case evbuf := <-r.ch:
			if (eventBytes + len(evbuf)) > maxEventBytes { // max buffer reached
				if len(evbuf) >= maxEventBytes {
					break // new event larger than max buffer size, drop
				}
				// drop oldest to make room for newest
				for dropped := 0; dropped < len(evbuf); {
					var oldest []byte
					oldest, batch = batch[0], batch[1:]
					dropped += len(oldest)
				}
			}
			// apend to batch
			batch = append(batch, evbuf)
			eventBytes += len(evbuf)
		case result := <-results:
			_ = result // XXX check return code, reconnect if disconnected
			logIsRunning = false
			flushBatch()
		case <-time.After(grpcReporterFlushTimeout):
			flushBatch()
		case <-r.exit:
			close(batches)
			break
		}
	}
}

func (r *grpcReporter) postEvents(batches <-chan [][]byte) <-chan *grpcResult {
	ret := make(chan *grpcResult)
	go func() {
		for batch := range batches {
			// call PostEvents
			req := &collector.MessageRequest{
				ApiKey:   r.apiKey,
				Messages: batch,
				Encoding: collector.EncodingType_BSON,
			}
			res, err := r.client.PostEvents(context.TODO(), req)
			ret <- &grpcResult{result: res, err: err}
		}
		close(ret)
	}()
	return ret
}

func (r *grpcReporter) PushMetricsRecord(record MetricsRecord) bool {
	if !r.IsOpen() {
		return false
	}
	return r.mAgg.PushMetricsRecord(&record)
}

// periodic is executed in a separate goroutine to encode messages and push them to the gRPC server
// This function is not concurrency-safe, don't run it in multiple goroutines.
func (r *grpcReporter) periodic() {
	go r.mAgg.ProcessMetrics()

	for {
		// Wait until next interval
		r.blockTillNextInterval(agentMetricsInterval)

		// send metrics
		r.sendMetrics()
		// send status
		r.sendStatus()
		// retrieve new settings
		r.getSettings()

		select {
		case <-r.exit:
			r.metrics.status = CLOSING
			break
		default:
		}

		r.healthCheck()
		if r.metrics.status == CLOSING {
			break
		}
	}
}

// healthCheck checks the status of the reporter (e.g., gRPC connection) and try to fix
// any problems found. It tries to close the reporter and release resources if the reporter is request to close.
func (r *grpcReporter) healthCheck() {
	// Close the reporter if requested.
	if r.metrics.status == CLOSING {
		r.closeMetricsConn()
		return
	}

	if r.metrics.status == OK {
		return
	} else { // disconnected
		r.reconnect()
	}

}

// reconnect is used to reconnect to the grpc server when the status is DISCONNECTED
// Consider using mutex as multiple goroutines will access the status parallelly
func (r *grpcReporter) reconnect() {
	OboeLog(DEBUG, "Reconnecting to gRPC server", nil)
	if r.metrics.status == OK {
		return
	} else if r.metrics.status == CLOSING {
		return
	} else {
		if r.metrics.retries > MaxRetriesNum {
			OboeLog(ERROR, "Reached retries limit, exiting", nil)
			r.metrics.status = CLOSING
			return
		}

		if r.metrics.nextRetryTime.After(time.Now()) {
			// check it again after 500ms, avoid comsuming too much CPU
			<-time.After(time.Millisecond * 500)
		} else { // reconnect
			conn, err := grpc.Dial(grpcReporterAddr) // TODO: is it the correct way to reconnect?
			if err != nil {
				r.metrics.nextRetryTime = time.Now().Add(time.Second * retryAmplifier) //TODO: reasonable?
				r.metrics.retries += 1
			} else { // reconnected
				r.metrics.client = collector.NewTraceCollectorClient(conn)
				r.metrics.retries = 0
				r.metrics.nextRetryTime = time.Time{}
				r.metrics.status = OK
			}

		}
	}
}

// Close request the reporter to quit from its goroutine by setting the exit flag
func (r *grpcReporter) RequestToClose() {
	r.exit <- struct{}{}
}

// close closes the channels and gRPC connections owned by a reporter
func (r *grpcReporter) closeMetricsConn() {
	// close channels and connections
	OboeLog(INFO, "periodic() metrics goroutine is exiting", nil)
	// Finally set toe reporter to nil to avoid repeated closing
	close(r.mAgg.GetExitChan())

	// TODO: close gRPC client
	r.metrics.client = nil
}

// blockTillNextInterval blocks the caller and will return at the next wake up time, which
// is the nearest multiple of interval (since the zero time)
func (r *grpcReporter) blockTillNextInterval(interval time.Duration) {
	if r.metrics.status != OK {
		return
	}

	now := time.Now()
	nextInterval := now.Round(interval)
	if nextInterval.Before(now) {
		nextInterval = nextInterval.Add(interval)
	}
	<-time.After(nextInterval.Sub(now))
}

// sendMetrics is called periodically (in a interval defined by agentMetricsInterval)
// to send metrics data to the gRPC sercer
func (r *grpcReporter) sendMetrics() {
	// Still need to fetch raw data from channel to avoid channels being filled with old data
	// (and possibly blocks the sender)
	messages, err := r.mAgg.FlushBSON()

	if err != nil || r.metrics.status != OK {
		return
	}

	mreq := &collector.MessageRequest{
		ApiKey:   r.apiKey,
		Messages: messages,
		Encoding: collector.EncodingType_BSON,
	}
	mres, err := r.metrics.client.PostMetrics(context.TODO(), mreq)
	_, _ = mres, err // TODO: error handling XXX

	return
}

// sendStatus is called periodically (in a interval defined by agentMetricsInterval)
// to send status events to the gRPC server.
func (r *grpcReporter) sendStatus() {
	// TODO: fetch data from status event channels before check status
	// TODO: (and drop it if reporter is DISCONNECTED

	if r.metrics.status != OK {
		return
	}
	// TODO: encode status message and call gRPC PostStatus to send it
}

// getSettings is called periodically (in a interval defined by agentMetricsInterval)
// to retrieve updated setting from gRPC server and process it.
func (r *grpcReporter) getSettings() {
	if r.metrics.status != OK {
		return
	}
	sreq := &collector.SettingsRequest{
		ApiKey:        r.apiKey,
		ClientVersion: grpcReporterVersion,
		Identity: &collector.HostID{
			Hostname:    cachedHostname,
			IpAddresses: nil, // XXX
			Uuid:        "",  // XXX
		},
	}
	sres, err := r.client.GetSettings(context.TODO(), sreq)
	if err != nil {
		return
	}
	storeSettings(sres) // TODO: settings
	return
	// TODO
}

func storeSettings(r *collector.SettingsResult) {
	if r != nil && len(r.Settings) > 0 {
		latestSettings = r.Settings
	}
}

func newGRPCReporter() reporter {
	// TODO: fetch data and release channel space even when gRPC is disconnected
	if reportingDisabled {
		return &nullReporter{}
	}
	conn, err := grpc.Dial(grpcReporterAddr)
	if err != nil {
		if os.Getenv("TRACEVIEW_DEBUG") != "" {
			log.Printf("TraceView failed to initialize gRPC reporter: %v", err)
		}
		return &nullReporter{}
	}
	r := &grpcReporter{
		client: collector.NewTraceCollectorClient(conn),
		ch:     make(chan []byte),
		exit:   make(chan struct{}),
		mAgg:   newMetricsAggregator(),
	}
	go r.reportEvents()
	go r.periodic() // metrics sender goroutine
	return r
}

var udpReporterAddr = "127.0.0.1:7831"
var grpcReporterAddr = "collector.librato.com:443"
var grpcReporterVersion = "golang-v1"
var globalReporter reporter = &nullReporter{}
var reportingDisabled bool
var usingTestReporter bool
var cachedHostname string
var debugLog bool
var debugLevel DebugLevel = ERROR
var latestSettings []*collector.OboeSetting

type hostnamer interface {
	Hostname() (name string, err error)
}
type osHostnamer struct{}

func (h osHostnamer) Hostname() (string, error) { return os.Hostname() }

func init() {
	debugLog = (os.Getenv("TRACEVIEW_DEBUG") != "")
	if addr := os.Getenv("TRACEVIEW_GRPC_COLLECTOR_ADDR"); addr != "" {
		grpcReporterAddr = addr
	}
	cacheHostname(osHostnamer{})
}
func cacheHostname(hn hostnamer) {
	h, err := hn.Hostname()
	if err != nil {
		if debugLog {
			log.Printf("Unable to get hostname, TraceView tracing disabled: %v", err)
		}
		globalReporter = &nullReporter{} // disable reporting
		reportingDisabled = true
	}
	cachedHostname = h
}

var cachedPid = os.Getpid()

func reportEvent(r reporter, ctx *oboeContext, e *event) error {
	if !r.IsOpen() {
		// Reporter didn't initialize, nothing to do...
		return nil
	}
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

	// Send BSON:
	bsonBufferFinish(&e.bbuf)
	_, err := r.WritePacket(e.bbuf.buf)
	return err
}

// Determines if request should be traced, based on sample rate settings:
// This is our only dependency on the liboboe C library.
func shouldTraceRequest(layer, xtraceHeader string) (sampled bool, sampleRate, sampleSource int) {
	return oboeSampleRequest(layer, xtraceHeader)
}

// PushMetricsRecord push the mAgg record into a channel using the global reporter.
func PushMetricsRecord(record MetricsRecord) bool {
	return globalReporter.PushMetricsRecord(record)
}
