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

	"fmt"
	"github.com/librato/go-traceview/v1/tv/internal/traceview/collector"
	"google.golang.org/grpc"
	"strings"
)

// Reporter status
const (
	OK           Status = iota
	DISCONNECTED        // will try to reconnect it later
	CLOSING             // closed by us, don't do anything to resume it
)

// Reporter parameters which are unlikely to change
// TODO: some of them may be updated by settings from server, don't use constants
const (
	maxEventBytes                 = 64 * 1024 * 1024
	grpcReporterFlushTimeout      = 100 * time.Millisecond
	agentMetricsInterval          = time.Minute
	agentMetricsTickInterval      = time.Millisecond * 500
	retryAmplifier                = 2
	initialRetryInterval          = time.Millisecond * 500
	maxRetryInterval              = time.Minute
	maxMetricsRetries             = 20
	maxConnRedirects              = 20
	maxConnRetries                = ^uint(0)
	maxStatusChanCap              = 200
	loadStatusMsgsShortBlock      = time.Millisecond * 5
	metricsConnKeepAliveInterval  = time.Second * 20
	maxMetricsMessagesOnePost     = 100
	agentSettingsInterval         = time.Second * 20
	agentCheckSettingsTTLInterval = time.Second * 10
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
	messages       [][]byte
	nextTime       time.Time
	retryActive    bool
	nextRetryDelay time.Duration
	retryTime      time.Time
	retries        uint
}

type gRPC struct {
	client            collector.TraceCollectorClient
	status            Status
	retries           uint
	nextRetryTime     time.Time // only works in DISCONNECTED state
	redirects         uint
	nextKeepAliveTime time.Time
	currTime          time.Time
}

type grpcReporter struct {
	client     collector.TraceCollectorClient
	serverAddr string // server address in string format: host:port
	exit       chan struct{}
	apiKey     string

	metricsConn               gRPC // metrics sender connection, for metrics, status and settings
	metrics, status, settings Sender
	ch                        chan []byte       // event messages
	mAgg                      MetricsAggregator // metrics raw records, need pre-processing
	sMsgs                     chan []byte       // status messages
}

type grpcResult struct {
	result *collector.MessageResult
	err    error
}

func (s *Sender) setRetryDelay(now time.Time) bool {
	if s.retries >= maxMetricsRetries {
		OboeLog(WARNING, "Maximum number of retries reached", nil)
		s.retryActive = false
		return false
	}
	s.retryTime = now.Add(s.nextRetryDelay)
	OboeLog(DEBUG, fmt.Sprintf("Retry in %d seconds", s.nextRetryDelay/time.Second))
	s.retries += 1
	if !s.retryActive {
		s.retryActive = true
	}
	s.nextRetryDelay *= retryAmplifier
	if s.nextRetryDelay > time.Minute {
		s.nextRetryDelay = time.Minute
	}
	return true
}

func (r *grpcReporter) IsOpen() bool            { return r.client != nil }
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

// TODO: add err print in critical paths -
// periodic is executed in a separate goroutine to encode messages and push them to the gRPC server
// This function is not concurrency-safe, don't run it in multiple goroutines.
func (r *grpcReporter) periodic() {
	OboeLog(DEBUG, "periodic(): goroutine started")
	go r.mAgg.ProcessMetrics()
	now := time.Now()
	// Initialize next keep alive time
	r.metricsConn.nextKeepAliveTime = getNextTime(now, metricsConnKeepAliveInterval)
	// Initialize next metric sending time
	r.metrics.nextTime = getNextTime(now, agentMetricsInterval)
	// Check and invalidate outdated settings
	var checkTTLTimeout = getNextTime(now, agentCheckSettingsTTLInterval)

	for {
		// avoid consuming too much CPU by sleeping for a short while.
		r.metricsConn.currTime = r.blockTillNextTick(time.Now(), agentMetricsTickInterval)
		// We still need to populate bson messages even if status is not OK.
		// populate and send metricsConn
		r.sendMetrics()
		// populate and send status
		r.sendStatus()
		// retrieve new settings
		r.getSettings()
		// invalidate outdated settings
		InvalidateOutdatedSettings(&checkTTLTimeout, r.metricsConn.currTime)
		// exit as per the request from the other (main) goroutine
		select {
		case <-r.exit:
			r.metricsConn.status = CLOSING
		default:
		}

		r.healthCheck()
		if r.metricsConnClosed() {
			// closed after health check, resources have been released.
			break // break the for loop
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
	OboeLog(DEBUG, "Reconnecting to gRPC server")
	if r.metricsConn.status == OK || r.metricsConn.status == CLOSING {
		return
	} else {
		if r.metricsConn.retries > maxConnRetries { // infinitely retry
			OboeLog(ERROR, "Reached retries limit, exiting")
			r.metricsConn.status = CLOSING // set it to CLOSING, it will be closed in the next loop
			return
		}

		if r.metricsConn.nextRetryTime.After(r.metricsConn.currTime) {
			// do nothing
		} else { // reconnect
			// TODO: close the old connection first, as we are redirecting ...
			conn, err := grpc.Dial(r.serverAddr) // TODO: correct way to reconnect? change addr to struct attribtue
			if err != nil {
				// TODO: retry time better to be exponential
				nextInterval := time.Second * time.Duration((r.metricsConn.retries+1)*retryAmplifier)
				if nextInterval > maxRetryInterval {
					nextInterval = maxRetryInterval
				}
				r.metricsConn.nextRetryTime = r.metricsConn.currTime.Add(nextInterval) // TODO: round up?
				r.metricsConn.retries += 1
			} else { // reconnected
				r.metricsConn.client = collector.NewTraceCollectorClient(conn)
				r.metricsConn.retries = 0
				r.metricsConn.nextRetryTime = time.Time{}
				r.metricsConn.status = OK
				r.metricsConn.nextKeepAliveTime = getNextTime(r.metricsConn.currTime, metricsConnKeepAliveInterval)
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
	if r.metricsConn.client == nil {
		OboeLog(WARNING, "closeMetricsConn(): closing a closed connection.")
		return
	}
	// close channels and connections
	OboeLog(INFO, "closeMetricsConn() closing metrics gRPC connection.")
	// Finally set toe reporter to nil to avoid repeated closing
	close(r.mAgg.GetExitChan())

	// TODO: close gRPC client
	r.metricsConn.client = nil
	r.metrics.messages = nil
	// TODO: set status/settings messages (if any) to nil
}

// blockTillNextTick blocks the caller and will return at the next wake up time, which
// is the nearest multiple of interval (since the zero time)
func (r *grpcReporter) blockTillNextTick(now time.Time, interval time.Duration) (curr time.Time) {
	// skip it if metricsConn connection is not working.
	if r.metricsConn.status != OK {
		return now
	}
	afterBlock := getNextTime(now, interval)
	<-time.After(afterBlock.Sub(now))
	return afterBlock
}

func getNextTime(now time.Time, interval time.Duration) time.Time {
	nextTime := now.Round(interval)
	if nextTime.Before(now) {
		nextTime = nextTime.Add(interval)
	}
	return nextTime
}

// sendMetrics is called periodically (in a interval defined by agentMetricsInterval)
// to send metricsConn data to the gRPC sercer
func (r *grpcReporter) sendMetrics() {
	// Still need to fetch raw data from channel to avoid channels being filled with old data
	// (and possibly blocks the sender)
	if r.metrics.nextTime.Before(r.metricsConn.currTime) {
		r.metrics.nextTime = getNextTime(r.metricsConn.currTime, agentMetricsInterval) // TODO: change to a value configured by settings.args

		message, err := r.mAgg.FlushBSON()
		if err == nil {
			r.metrics.messages = append(r.metrics.messages, message)
			if len(r.metrics.messages) > maxMetricsMessagesOnePost {
				r.metrics.messages = r.metrics.messages[1:]
			}
		}
	}
	// return if in retry state but it's not time for retry
	if r.metrics.retryActive && r.metrics.retryTime.After(r.metricsConn.currTime) {
		return
	}
	// return if connection is not OK or we have no message to send
	if r.metricsConn.status != OK || len(r.metrics.messages) == 0 {
		return
	}
	// OK we are good now.
	mreq := &collector.MessageRequest{
		ApiKey:   r.apiKey,
		Messages: r.metrics.messages,
		Encoding: collector.EncodingType_BSON,
	}
	mres, err := r.metricsConn.client.PostMetrics(context.TODO(), mreq)
	if err != nil {
		OboeLog(INFO, "Error in sending metrics", err)
		r.metricsConn.status = DISCONNECTED // TODO: is this process correct?
		return
	}
	// Update connection keep alive time
	r.metricsConn.nextKeepAliveTime = getNextTime(r.metricsConn.currTime, metricsConnKeepAliveInterval)

	switch result := mres.GetResult(); result {
	case collector.ResultCode_OK:
		OboeLog(DEBUG, "sendMetrics(): sent metrics.")
		r.metrics.messages = make([][]byte, 0, 1)
		r.metrics.retries = 0
		r.metrics.retryActive = false
		r.metricsConn.redirects = 0
	case collector.ResultCode_TRY_LATER, collector.ResultCode_LIMIT_EXCEEDED:
		msg := fmt.Sprintf("sendMetrics(): got %s from server", collector.ResultCode_name[int32(result)])
		OboeLog(INFO, msg)
		if r.metrics.setRetryDelay(r.metricsConn.currTime) {
			r.metrics.messages = r.metrics.messages[1:] // TODO: correct?
		}
	case collector.ResultCode_INVALID_API_KEY:
		OboeLog(WARNING, "sendMetrics(): got INVALID_API_KEY from server")
		r.metricsConn.status = CLOSING
		r.metrics.messages = nil // connection is closing so we're OK with nil
	case collector.ResultCode_REDIRECT:
		r.processRedirect(mres.GetArg())
	}
	return
}

func (r *grpcReporter) setServerAddr(host string) bool {
	//TODO: create a new attribute 'server' for grpcReporter
	if strings.Contains(host, ":") {
		OboeLog(WARNING, fmt.Sprintf("Invalid reporter server address: %s", host))
		return false
	} else {
		// TODO: mutex protection
		// TODO: further IP validity check.
		r.serverAddr = host
		return true
	}

}

// TODO: need an API to the trace to send status message (check grpc is ready otherwise return)

// sendStatus is called periodically (in a interval defined by agentMetricsInterval)
// to send status events to the gRPC server.
func (r *grpcReporter) sendStatus() {
	if r.metricsConn.status != OK {
		return
	}
	// return if we're retrying and it's not time for retry
	if r.status.retryActive && r.status.retryTime.After(r.metricsConn.currTime) { // TODO: double check
		return
	}

	if len(r.status.messages) > 0 || r.loadStatusMsgs() {
		mreq := &collector.MessageRequest{
			ApiKey:   r.apiKey,
			Messages: r.status.messages,
			Encoding: collector.EncodingType_BSON,
		}
		mres, err := r.metricsConn.client.PostStatus(context.TODO(), mreq)
		if err != nil {
			OboeLog(INFO, "Error in sending metrics", err)
			r.metricsConn.status = DISCONNECTED // TODO: is this process correct?
			return
		}
		// Update connection keep alive time
		r.metricsConn.nextKeepAliveTime = getNextTime(r.metricsConn.currTime, metricsConnKeepAliveInterval)

		switch result := mres.GetResult(); result {
		case collector.ResultCode_OK:
			OboeLog(DEBUG, "sendMetrics(): sent status")
			r.status.messages = make([][]byte, 0, 1)
			r.status.retryActive = false
			r.metricsConn.redirects = 0
		case collector.ResultCode_TRY_LATER, collector.ResultCode_LIMIT_EXCEEDED:
			msg := fmt.Sprintf("sendMetrics(): got %s from server", collector.ResultCode_name[int32(result)])
			OboeLog(INFO, msg)
			if r.status.setRetryDelay(r.metricsConn.currTime) {
				r.status.messages = make([][]byte, 0, 1)
			}
		case collector.ResultCode_INVALID_API_KEY:
			OboeLog(WARNING, "sendMetrics(): got INVALID_API_KEY from server")
			r.metricsConn.status = CLOSING
			r.status.messages = nil // connection is closing so we're OK with nil
		case collector.ResultCode_REDIRECT:
			r.processRedirect(mres.GetArg())
		}
	}
}

// processRedirect process the redirect response from server and set the new server address
func (r *grpcReporter) processRedirect(host string) {
	if r.metricsConn.redirects >= maxConnRedirects {
		OboeLog(WARNING, "Maximum redirects reached, exiting")
		r.metricsConn.status = CLOSING
	} else {
		r.metricsConn.status = DISCONNECTED
		if r.setServerAddr(host) {
			r.metrics.retryActive = false
			r.metricsConn.redirects += 1
			r.metricsConn.retries = 0
			r.metricsConn.nextRetryTime = time.Time{}
		} else {
			r.metricsConn.status = CLOSING
		}
	}
}

// TODO: API for sending and encoding status message

// loadStatusMsgs loads messages from reporter's sMsgs channel to the status senders
// messages slice, the messages will be sent out in current loop
func (r *grpcReporter) loadStatusMsgs() bool {
	var sMsg []byte
loop:
	for {
		select {
		case sMsg = <-r.sMsgs:
			r.status.messages = append(r.status.messages, sMsg)
		case <-time.After(loadStatusMsgsShortBlock):
			break loop
		}
	}
	return len(r.status.messages) > 0
}

// getSettings is called periodically (in a interval defined by agentMetricsInterval)
// to retrieve updated setting from gRPC server and process it.
func (r *grpcReporter) getSettings() { // TODO: use it as keep alive msg
	if r.metricsConn.status != OK {
		return
	}

	tn := r.metricsConn.currTime
	if (!r.settings.retryActive &&
		(r.settings.nextTime.Before(tn) || r.metricsConn.nextKeepAliveTime.Before(tn))) ||
		r.settings.retryTime.Before(tn) {
		OboeLog(DEBUG, "getSettings(): updating settings")
		mAgg, ok := r.mAgg.(metricsAggregator)
		var ipAddrs []string
		var uuid string
		if ok {
			ipAddrs = mAgg.getIPList()
			uuid = mAgg.getHostId()
		} else {
			ipAddrs = nil
			uuid = ""
		}
		sreq := &collector.SettingsRequest{
			ApiKey:        r.apiKey,
			ClientVersion: grpcReporterVersion,
			Identity: &collector.HostID{
				Hostname:    cachedHostname,
				IpAddresses: ipAddrs,
				Uuid:        uuid,
			},
		}
		sres, err := r.client.GetSettings(context.TODO(), sreq)
		if err != nil {
			OboeLog(INFO, "Error in retrieving settings", err)
			r.metricsConn.status = DISCONNECTED // TODO: is this process correct?
			return
		}
		r.metricsConn.nextKeepAliveTime = getNextTime(r.metricsConn.currTime, metricsConnKeepAliveInterval)

		switch result := sres.GetResult(); result {
		case collector.ResultCode_OK:
			OboeLog(DEBUG, "getSettings(): got new settings from server")
			storeSettings(sres)
			r.settings.nextTime = getNextTime(r.metricsConn.currTime, agentSettingsInterval)
			r.settings.retryActive = false
			r.metricsConn.redirects = 0
		case collector.ResultCode_TRY_LATER, collector.ResultCode_LIMIT_EXCEEDED:
			msg := fmt.Sprintf("sendMetrics(): got %s from server", collector.ResultCode_name[int32(result)])
			OboeLog(INFO, msg)
			r.settings.retries = 0 // retry infinitely
			r.settings.setRetryDelay(r.metricsConn.currTime)

		case collector.ResultCode_INVALID_API_KEY:
			OboeLog(DEBUG, "sendMetrics(): got INVALID_API_KEY, exiting")
			r.metricsConn.status = CLOSING
		case collector.ResultCode_REDIRECT:
			r.processRedirect(sres.GetArg())
		}
	}
}

// TODO: update settings
func storeSettings(r *collector.SettingsResult) {
	if r != nil && len(r.Settings) > 0 {
		latestSettings = r.Settings
	}
}

// TODO:
func InvalidateOutdatedSettings(timeout *time.Time, curr time.Time) {
	if timeout.Before(curr) {
		// TODO: delete outdated settings
		*timeout = getNextTime(curr, agentCheckSettingsTTLInterval)
	}
}

func newSender() Sender {
	return Sender{
		messages:       make([][]byte, 0, 1),
		nextTime:       time.Time{},
		retryActive:    false,
		nextRetryDelay: initialRetryInterval,
		retryTime:      time.Time{},
		retries:        0,
	}
}

func newGRPC(conn *grpc.ClientConn) gRPC {
	return gRPC{
		client:            collector.TraceCollectorClient(conn),
		status:            OK,
		retries:           0,
		nextRetryTime:     time.Time{},
		redirects:         0,
		nextKeepAliveTime: time.Time{},
		currTime:          time.Time{},
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
		OboeLog(ERROR, "Failed to intialize gRPC metrics reporter")
		// TODO: close conn connection
		return &nullReporter{}
	}

	r := &grpcReporter{
		client:      collector.NewTraceCollectorClient(conn),
		serverAddr:  grpcReporterAddr,
		metricsConn: newGRPC(mConn),
		metrics:     newSender(),
		status:      newSender(),
		settings:    newSender(),
		ch:          make(chan []byte),
		exit:        make(chan struct{}),
		mAgg:        newMetricsAggregator(),
		sMsgs:       make(chan []byte, maxStatusChanCap),
	}
	go r.reportEvents()
	go r.periodic() // metricsConn sender goroutine
	return r
}

var udpReporterAddr = "127.0.0.1:7831"
var grpcReporterAddr = "collector.librato.com:443"
var grpcReporterVersion = "golang-v2"
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
