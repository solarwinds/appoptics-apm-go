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
	RECONNECTING        // reconnecting to gRPC server (may be a redirected one)
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
	agentMetricsStepInterval = time.Millisecond * 500
	retryAmplifier           = 2
	maxRetryInterval         = 60
)

type reporter interface {
	WritePacket([]byte) (int, error)
	IsOpen() bool
	IsMetricsConnOpen() bool
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
func (r *nullReporter) IsMetricsConnOpen() bool                     { return false }
func (r *nullReporter) WritePacket(buf []byte) (int, error)         { return len(buf), nil }
func (r *nullReporter) PushMetricsRecord(record MetricsRecord) bool { return true }

type udpReporter struct {
	conn *net.UDPConn
}

func (r *udpReporter) IsOpen() bool                                { return r.conn != nil }
func (r *udpReporter) IsMetricsConnOpen() bool                     { return false }
func (r *udpReporter) WritePacket(buf []byte) (int, error)         { return r.conn.Write(buf) }
func (r *udpReporter) PushMetricsRecord(record MetricsRecord) bool { return false }

type Status int

type Sender struct {
	messages    [][]byte
	nextTime    time.Time
	retryActive bool
	retryDelay  time.Time
	retryTime   time.Time
	retries     uint
}

type gRPC struct {
	client        collector.TraceCollectorClient
	status        Status
	retries       uint
	nextRetryTime time.Time
}

type grpcReporter struct {
	client      collector.TraceCollectorClient
	metricsConn gRPC
	metrics     Sender
	status      Sender
	settings    Sender
	ch          chan []byte
	exit        chan struct{}
	apiKey      string
	mAgg        MetricsAggregator
}

type grpcResult struct {
	result *collector.MessageResult
	err    error
}

func (r *grpcReporter) IsOpen() bool            { return r.client != nil } // TODO
func (r *grpcReporter) IsMetricsConnOpen() bool { return r.metricsConn.client != nil }
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
	if !r.IsMetricsConnOpen() {
		return false
	}
	return r.mAgg.PushMetricsRecord(&record)
}

// periodic is executed in a separate goroutine to encode messages and push them to the gRPC server
// This function is not concurrency-safe, don't run it in multiple goroutines.
func (r *grpcReporter) periodic() {
	OboeLog(DEBUG, "periodic(): goroutine started", nil)
	go r.mAgg.ProcessMetrics()

	// Initialize next metric sending time
	r.metrics.nextTime = getNextIntervalTime(agentMetricsInterval)
	for {
		// avoid consuming too much CPU by sleeping for a short while.
		r.blockTillNextInterval(agentMetricsStepInterval)

		if r.metricsConn.status == OK {
			// send metricsConn
			r.sendMetrics()
			// send status
			r.sendStatus()
			// retrieve new settings
			r.getSettings()
		}
		// exit as per the request from the other (main) goroutine
		select {
		case <-r.exit:
			r.metricsConn.status = CLOSING
			break
		default:
		}

		r.healthCheck()
		if r.metricsConnClosed() {
			// closed after health check, resources have been released.
			break
		}
	}
}

// metricsConnClosed checks if the metrics sending connection is closed
func (r *grpcReporter) metricsConnClosed() bool {
	return r.metricsConn.status == CLOSING && r.metricsConn.client == nil
}

// healthCheck checks the status of the reporter (e.g., gRPC connection) and try to fix
// any problems found. It tries to close the reporter and release resources if the
// reporter is request to close.
func (r *grpcReporter) healthCheck() {
	if r.metricsConn.status == OK {
		return
	}
	// Close the reporter if requested.
	if r.metricsConn.status == CLOSING {
		r.closeMetricsConn()
		return
	} else { // disconnected or reconnecting (check retry timeout)
		r.reconnect()
	}

}

// reconnect is used to reconnect to the grpc server when the status is DISCONNECTED
// Consider using mutex as multiple goroutines will access the status parallelly
func (r *grpcReporter) reconnect() {
	// TODO: gRPC supports auto-reconnection, need to make sure what happens to the sending API then,
	// TODO: does it wait for the reconnection, or it returns an error immediately?
	OboeLog(DEBUG, "Reconnecting to gRPC server", nil)
	if r.metricsConn.status == OK || r.metricsConn.status == CLOSING {
		return
	} else {
		if r.metricsConn.retries > MaxRetriesNum { // infinitely retry
			OboeLog(ERROR, "Reached retries limit, exiting", nil)
			r.metricsConn.status = CLOSING // set it to CLOSING, it will be closed in the next loop
			return
		}

		if r.metricsConn.nextRetryTime.After(time.Now()) {
			// do nothing
		} else { // reconnect
			// TODO: close the old connection first, as we are redirecting ...
			conn, err := grpc.Dial(grpcReporterAddr) // TODO: correct way to reconnect? change addr to struct attribtue
			if err != nil {
				// TODO: retry time better to be exponential
				nextInterval := (r.metricsConn.retries + 1) * retryAmplifier
				if nextInterval > maxRetryInterval {
					nextInterval = maxRetryInterval
				}
				r.metricsConn.nextRetryTime = time.Now().Add(time.Second * time.Duration(nextInterval)) // TODO: round up?
				r.metricsConn.retries += 1
			} else { // reconnected
				r.metricsConn.client = collector.NewTraceCollectorClient(conn)
				r.metricsConn.retries = 0
				r.metricsConn.nextRetryTime = time.Time{}
				r.metricsConn.status = OK
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
	OboeLog(INFO, "periodic() metricsConn goroutine is exiting", nil)
	// Finally set toe reporter to nil to avoid repeated closing
	close(r.mAgg.GetExitChan())

	// TODO: close gRPC client
	r.metricsConn.client = nil
}

// blockTillNextInterval blocks the caller and will return at the next wake up time, which
// is the nearest multiple of interval (since the zero time)
func (r *grpcReporter) blockTillNextInterval(interval time.Duration) {
	// skip the delay if metricsConn connection is not working.
	if r.metricsConn.status != OK {
		return
	}

	now := time.Now()
	nextInterval := now.Round(interval)
	if nextInterval.Before(now) {
		nextInterval = nextInterval.Add(interval)
	}
	<-time.After(getNextIntervalTime(interval).Sub(now))
}

func getNextIntervalTime(interval time.Duration) time.Time {
	now := time.Now()
	nextInterval := now.Round(interval)
	if nextInterval.Before(now) {
		nextInterval = nextInterval.Add(interval)
	}
	return nextInterval
}

// sendMetrics is called periodically (in a interval defined by agentMetricsInterval)
// to send metricsConn data to the gRPC sercer
func (r *grpcReporter) sendMetrics() {
	// Still need to fetch raw data from channel to avoid channels being filled with old data
	// (and possibly blocks the sender)
	if r.metrics.nextTime.After(time.Now()) && !r.metrics.retryActive {
		return
	}
	if r.metrics.nextTime.Before(time.Now()) {
		r.metrics.nextTime = getNextIntervalTime(agentMetricsInterval) // TODO: change to a value configured by settings.args

		message, err := r.mAgg.FlushBSON()
		if err == nil {
			r.metrics.messages = append(r.metrics.messages, message)
		}

	}
	if len(r.metrics.messages) == 0 || r.metricsConn.status != OK {
		return
	}

	mreq := &collector.MessageRequest{
		ApiKey:   r.apiKey,
		Messages: r.metrics.messages,
		Encoding: collector.EncodingType_BSON,
	}
	mres, err := r.metricsConn.client.PostMetrics(context.TODO(), mreq)
	if err != nil {
		OboeLog(INFO, "Error in sending metrics", err)
		r.metricsConn.status = DISCONNECTED
		return
	}
	switch mres.GetResult() {
	case collector.ResultCode_OK:
		OboeLog(DEBUG, "Sent metrics.", nil)
		r.metrics.messages = make([][]byte, 1)
		r.metrics.retries = 0
		r.metrics.retryActive = false
	case collector.ResultCode_TRY_LATER:
		OboeLog(WARNING, "sendMetrics(): got TRY_LATER from gRPC server", nil)
		if r.metrics.retries >= MaxRetriesNum {
			OboeLog(WARNING, "sendMetrics(): exceeds max number of retries", nil)
			r.metrics.messages = r.metrics.messages[1:] // TODO: correct?
			break
		}
		r.updateRetryParms()
	case collector.ResultCode_LIMIT_EXCEEDED:
		// TODO
	case collector.ResultCode_INVALID_API_KEY:
		// TODO
	case collector.ResultCode_REDIRECT:
		// TODO

	}
	return
}

// updateRetryParms increases retries number, moves to the next level of retry delay, etc.
func (r *grpcReporter) updateRetryParms() {
	// TODO
}

// sendStatus is called periodically (in a interval defined by agentMetricsInterval)
// to send status events to the gRPC server.
func (r *grpcReporter) sendStatus() {
	// TODO: fetch data from status event channels before check status
	// TODO: (and drop it if reporter is DISCONNECTED

	if r.metricsConn.status != OK {
		return
	}
	// TODO: encode status message and call gRPC PostStatus to send it
}

// getSettings is called periodically (in a interval defined by agentMetricsInterval)
// to retrieve updated setting from gRPC server and process it.
func (r *grpcReporter) getSettings() {
	if r.metricsConn.status != OK {
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

func newSender() Sender {
	return Sender{
		messages:    make([][]byte, 1),
		nextTime:    time.Time{},
		retryActive: false,
		retryDelay:  time.Time{},
		retryTime:   time.Time{},
		retries:     0,
	}
}

func newGRPC(conn *grpc.ClientConn) gRPC {
	return gRPC{
		client:        collector.TraceCollectorClient(conn),
		status:        OK,
		retries:       0,
		nextRetryTime: time.Time{},
	}
}

func newGRPCReporter() reporter {
	// TODO: fetch data and release channel space even when gRPC is disconnected
	if reportingDisabled {
		return &nullReporter{}
	}
	conn, err := grpc.Dial(grpcReporterAddr)
	if err != nil {
		if os.Getenv("TRACEVIEW_DEBUG") != "" { // TODO: use OboeLog
			log.Printf("TraceView failed to initialize gRPC reporter: %v", err)
		}
		return &nullReporter{}
	}
	mConn, err := grpc.Dial(grpcReporterAddr) // TODO: can we have two connections?
	if err != nil {
		OboeLog(ERROR, "Failed to intialize gRPC metrics reporter", nil)
		// TODO: close conn connection
		return &nullReporter{}
	}

	r := &grpcReporter{
		client:      collector.NewTraceCollectorClient(conn),
		metricsConn: newGRPC(mConn),
		metrics:     newSender(),
		status:      newSender(),
		settings:    newSender(),
		ch:          make(chan []byte),
		exit:        make(chan struct{}),
		mAgg:        newMetricsAggregator(),
	}
	go r.reportEvents()
	go r.periodic() // metricsConn sender goroutine
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
