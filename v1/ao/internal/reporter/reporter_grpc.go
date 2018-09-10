// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io/ioutil"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/pkg/errors"
	"google.golang.org/grpc/credentials"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/host"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	grpcReporterVersion = "golang-v2"

	// default certificate used to verify the collector endpoint,
	// can be overridden via APPOPTICS_TRUSTEDPATH
	grpcCertDefault = `-----BEGIN CERTIFICATE-----
MIID8TCCAtmgAwIBAgIJAMoDz7Npas2/MA0GCSqGSIb3DQEBCwUAMIGOMQswCQYD
VQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNU2FuIEZyYW5j
aXNjbzEVMBMGA1UECgwMTGlicmF0byBJbmMuMRUwEwYDVQQDDAxBcHBPcHRpY3Mg
Q0ExJDAiBgkqhkiG9w0BCQEWFXN1cHBvcnRAYXBwb3B0aWNzLmNvbTAeFw0xNzA5
MTUyMjAxMzlaFw0yNzA5MTMyMjAxMzlaMIGOMQswCQYDVQQGEwJVUzETMBEGA1UE
CAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNU2FuIEZyYW5jaXNjbzEVMBMGA1UECgwM
TGlicmF0byBJbmMuMRUwEwYDVQQDDAxBcHBPcHRpY3MgQ0ExJDAiBgkqhkiG9w0B
CQEWFXN1cHBvcnRAYXBwb3B0aWNzLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEP
ADCCAQoCggEBAOxO0wsGba3iI4r3L5BMST0rAO/gGaUhpQre6nRwVTmPCnLw1bmn
GdiFgYv/oRRwU+VieumHSQqoOmyFrg+ajGmvUDp2WqQ0It+XhcbaHFiAp2H7+mLf
cUH6S43/em0WUxZHeRzRupRDyO1bX6Hh2jgxykivlFrn5HCIQD5Hx1/SaZoW9v2n
oATCbgFOiPW6kU/AVs4R0VBujon13HCehVelNKkazrAEBT1i6RvdOB6aQQ32seW+
gLV5yVWSPEJvA9ZJqad/nQ8EQUMSSlVN191WOjp4bGpkJE1svs7NmM+Oja50W56l
qOH5eWermr/8qWjdPlDJ+I0VkgN0UyHVuRECAwEAAaNQME4wHQYDVR0OBBYEFOuL
KDTFhRQXwlBRxhPqhukrNYeRMB8GA1UdIwQYMBaAFOuLKDTFhRQXwlBRxhPqhukr
NYeRMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAJQtH446NZhjusy6
iCyvmnD95ybfNPDpjHmNx5n9Y6w9n+9y1o3732HUJE+WjvbLS3h1o7wujGKMcRJn
7I7eTDd26ZhLvnh5/AitYjdxrtUkQDgyxwLFJKhZu0ik2vXqj0fL961/quJL8Gyp
hNj3Nf7WMohQMSohEmCCX2sHyZGVGYmQHs5omAtkH/NNySqmsWNcpgd3M0aPDRBZ
5VFreOSGKBTJnoLNqods/S9RV0by84hm3j6aQ/tMDIVE9VCJtrE6evzC0MWyVFwR
ftgwcxyEq5SkiR+6BCwdzAMqADV37TzXDHLjwSrMIrgLV5xZM20Kk6chxI5QAr/f
7tsqAxw=
-----END CERTIFICATE-----`

	// These are hard-coded parameters for the gRPC reporter. Any of them become
	// configurable in future versions will be moved to package config.
	// TODO: use time.Time
	grpcMetricIntervalDefault               = 30               // default metrics flush interval in seconds
	grpcGetSettingsIntervalDefault          = 30               // default settings retrieval interval in seconds
	grpcSettingsTimeoutCheckIntervalDefault = 10               // default check interval for timed out settings in seconds
	grpcPingIntervalDefault                 = 20               // default interval for keep alive pings in seconds
	grpcRetryDelayInitial                   = 500              // initial connection/send retry delay in milliseconds
	grpcRetryDelayMultiplier                = 1.5              // backoff multiplier for unsuccessful retries
	grpcRetryDelayMax                       = 60               // max connection/send retry delay in seconds
	grpcCtxTimeout                          = 10 * time.Second // gRPC method invocation timeout in seconds
	grpcRedirectMax                         = 20               // max allowed collector redirects
	grpcRetryLogThreshold                   = 10               // log prints after this number of retries (about 56.7s)
	grpcMaxRetries                          = 20               // The message will be dropped after this number of retries
)

type reporterChannel int

// a channel the reporter is listening on for messages from the agent
const (
	EVENTS reporterChannel = iota
	METRICS
)

// everything needed for a GRPC connection
type grpcConnection struct {
	name           string                         // connection name
	client         collector.TraceCollectorClient // GRPC client instance
	connection     *grpc.ClientConn               // GRPC connection object
	address        string                         // collector address
	certificate    []byte                         // collector certificate
	pingTicker     *time.Timer                    // timer for keep alive pings in seconds
	pingTickerLock sync.Mutex                     // lock to ensure sequential access of pingTicker
	lock           sync.RWMutex                   // lock to ensure sequential access (in case of connection loss)
	queueStats     *eventQueueStats               // queue stats (reset on each metrics report cycle)
	// for testing only: if true, skip verifying TLS cert hostname
	insecureSkipVerify bool
	// atomicActive indicates if the underlying connection is active. It should
	// be reconnected or redirected to a new address in case of inactive. The
	// value 0 represents false and a value other than 0 (usually 1) means true
	atomicActive int32

	// the backoff function
	backoff Backoff
	Dialer
}

// GrpcConnOpt defines the function type that sets an option of the grpcConnection
type GrpcConnOpt func(c *grpcConnection)

// WithCert returns a function that sets the certificate
func WithCert(cert []byte) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.certificate = cert
	}
}

// WithSkipVerify returns a function that sets the insecureSkipVerify option
func WithSkipVerify(skip bool) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.insecureSkipVerify = skip
	}
}

// WithDialer returns a function that sets the Dialer option
func WithDialer(d Dialer) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.Dialer = d
	}
}

// WithBackoff return a function that sets the backoff option
func WithBackoff(b Backoff) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.backoff = b
	}
}

func newGrpcConnection(name string, target string, opts ...GrpcConnOpt) (*grpcConnection, error) {
	gc := &grpcConnection{
		name:               name,
		client:             nil,
		connection:         nil,
		address:            target,
		certificate:        []byte(grpcCertDefault),
		queueStats:         &eventQueueStats{},
		insecureSkipVerify: false,
		backoff:            DefaultBackoff,
		Dialer:             &DefaultDialer{},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(gc)
		}
	}

	err := gc.connect()
	if err != nil {
		return nil, errors.Wrap(err, name)
	}
	return gc, nil
}

// Close closes the gRPC connection and set the pointer to nil
func (c *grpcConnection) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.connection != nil {
		c.connection.Close()
	}
	c.connection = nil
}

type grpcReporter struct {
	eventConnection              *grpcConnection // used for events only
	metricConnection             *grpcConnection // used for everything else (postMetrics, postStatus, getSettings)
	collectMetricInterval        int             // metrics flush interval in seconds
	getSettingsInterval          int             // settings retrieval interval in seconds
	settingsTimeoutCheckInterval int             // check interval for timed out settings in seconds
	collectMetricIntervalLock    sync.RWMutex    // lock to ensure sequential access of collectMetricInterval

	serviceKey string // service key

	eventMessages  chan []byte      // channel for event messages (sent from agent)
	spanMessages   chan SpanMessage // channel for span messages (sent from agent)
	statusMessages chan []byte      // channel for status messages (sent from agent)
	metricMessages chan []byte      // channel for metrics messages (internal to reporter)

	// The reporter is ready to use only after this channel is closed
	ready chan struct{}
	// The channel should only be closed once
	readyOnce sync.Once

	// The reporter doesn't have a explicit field to record its state. This channel is used to notify all the
	// long-running goroutines to stop themselves. All the goroutines will check this channel and close if the
	// channel is closed.
	// Don't send data into this channel, just close it by calling Shutdown().
	done       chan struct{}
	doneClosed sync.Once
}

// gRPC reporter errors
var (
	ErrShutdownClosedReporter = errors.New("trying to shutdown a closed reporter")
	ErrReporterIsClosed       = errors.New("the reporter is closed")
	ErrMaxRetriesExceeded     = errors.New("maximum retries exceeded")
)

// initializes a new GRPC reporter from scratch (called once on program startup)
//
// returns	GRPC reporter object
func newGRPCReporter() reporter {
	if reportingDisabled {
		return &nullReporter{}
	}

	// service key is required, so bail out if not found
	serviceKey := config.GetServiceKey()
	if !config.IsValidServiceKey(serviceKey) {
		log.Errorf("Invalid service key (token:serviceName): <%s>. Reporter disabled.", serviceKey)
		log.Error("Check AppOptics dashboard for your token and use a service name shorter than 256 characters.")
		return &nullReporter{}
	}

	// collector address override
	addr := config.GetCollector()

	var opts []GrpcConnOpt
	// certificate override
	if certPath := config.GetTrustedPath(); certPath != "" {
		var err error
		cert, err := ioutil.ReadFile(certPath)
		if err != nil {
			log.Errorf("Error reading cert file %s: %v", certPath, err)
			return &nullReporter{}
		}
		opts = append(opts, WithCert(cert))
	}

	opts = append(opts, WithSkipVerify(config.GetSkipVerify()))

	// create connection object for events client and metrics client
	eventConn, err1 := newGrpcConnection("events channel", addr, opts...)
	if err1 != nil {
		log.Errorf("Failed to initialize gRPC reporter %v: %v", addr, err1)
		return &nullReporter{}
	}
	metricConn, err2 := newGrpcConnection("metrics channel", addr, opts...)
	if err2 != nil {
		eventConn.Close()
		log.Errorf("Failed to initialize gRPC reporter %v: %v", addr, err2)
		return &nullReporter{}
	}

	// construct the reporter object which handles two connections
	r := &grpcReporter{
		eventConnection:  eventConn,
		metricConnection: metricConn,

		collectMetricInterval:        grpcMetricIntervalDefault,
		getSettingsInterval:          grpcGetSettingsIntervalDefault,
		settingsTimeoutCheckInterval: grpcSettingsTimeoutCheckIntervalDefault,

		serviceKey: serviceKey,

		eventMessages:  make(chan []byte, 10000),
		spanMessages:   make(chan SpanMessage, 1024),
		statusMessages: make(chan []byte, 1024),
		metricMessages: make(chan []byte, 1024),

		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}

	r.start()

	log.Infof("AppOptics reporter is started: %v", r.done)
	return r
}

func (r *grpcReporter) start() {
	// start up long-running goroutine eventSender() which listens on the events message channel
	// and reports incoming events to the collector using GRPC
	go r.eventSender()

	// start up long-running goroutine statusSender() which listens on the status message channel
	// and reports incoming events to the collector using GRPC
	go r.statusSender()

	// start up long-running goroutine periodicTasks() which kicks off periodic tasks like
	// collectMetrics() and getSettings()
	if !periodicTasksDisabled {
		go r.periodicTasks()
	}

	// start up long-running goroutine spanMessageAggregator() which listens on the span message
	// channel and processes incoming span messages
	go r.spanMessageAggregator()
}

// Shutdown closes the reporter by close the `done` channel. All long-running goroutines
// monitor the channel `done` in the reporter and close themselves when the channel is closed.
func (r *grpcReporter) Shutdown() error {
	select {
	case <-r.done:
		return ErrShutdownClosedReporter
	default:
		r.doneClosed.Do(func() {
			log.Warningf("Shutting down the gRPC reporter: %v", r.done)
			close(r.done)
			// There may be messages
			r.closeConns()
			host.Stop()
		})
		return nil
	}
}

// closeConns closes all the gRPC connections of a reporter
func (r *grpcReporter) closeConns() {
	r.eventConnection.Close()
	r.metricConnection.Close()
}

// Closed return true if the reporter is already closed, or false otherwise.
func (r *grpcReporter) Closed() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

func (r *grpcReporter) setReady() {
	r.readyOnce.Do(func() {
		close(r.ready)
	})
}

// IsReady checks the state of the reporter and may wait for up to the specified
// duration until it becomes ready.
//
// The reporter is still considered `not ready` if (in rare cases) the default
// setting is retrieved from the collector but expires after the TTL, and no new
// setting is fetched.
func (r *grpcReporter) IsReady(timeout time.Duration) bool {
	select {
	case <-r.ready:
		return hasDefaultSetting()
	default:
	}

	select {
	case <-r.ready:
		return hasDefaultSetting()
	case <-time.After(timeout):
		return false
	}
}

func (c *grpcConnection) setAddress(addr string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.address = addr
	c.setActive(false)
}

// connect does the operation of connecting to a collector. It may be the same
// address or a new one. Those who issue the connection request need to set
// the stale flag to true, otherwise this function will do nothing.
func (c *grpcConnection) connect() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Skip it if the connection is not stale - someone else may have done
	// the connection.
	if c.isActive() {
		log.Debug("[%s] Someone else has done the redirection.", c.name)
		return nil
	}
	// create a new connection object for this client
	conn, err := c.Dial(*c)
	if err != nil {
		return errors.Wrap(err, "failed to connect to target")
	}

	// close the old connection
	if c.connection != nil {
		c.connection.Close()
	}
	// set new connection (need to be protected)
	c.connection = conn
	c.client = collector.NewTraceCollectorClient(c.connection)
	c.setActive(true)

	log.Infof("[%s] Connected to %s", c.name, c.address)
	return nil
}

func (c *grpcConnection) isActive() bool {
	return atomic.LoadInt32(&c.atomicActive) == 1
}

func (c *grpcConnection) setActive(active bool) {
	var flag int32
	if active {
		flag = 1
	}
	atomic.StoreInt32(&c.atomicActive, flag)
}

func (c *grpcConnection) reconnect() {
	c.connect()
}

// long-running goroutine that kicks off periodic tasks like collectMetrics() and getSettings()
func (r *grpcReporter) periodicTasks() {
	defer log.Warning("periodicTasks goroutine exiting.")

	// set up tickers
	collectMetricsTicker := time.NewTimer(r.collectMetricsNextInterval())
	getSettingsTicker := time.NewTimer(0)
	settingsTimeoutCheckTicker := time.NewTimer(time.Duration(r.settingsTimeoutCheckInterval) * time.Second)
	r.eventConnection.pingTicker = time.NewTimer(time.Duration(grpcPingIntervalDefault) * time.Second)
	r.metricConnection.pingTicker = time.NewTimer(time.Duration(grpcPingIntervalDefault) * time.Second)

	defer func() {
		collectMetricsTicker.Stop()
		getSettingsTicker.Stop()
		settingsTimeoutCheckTicker.Stop()
		r.eventConnection.pingTicker.Stop()
		r.metricConnection.pingTicker.Stop()
	}()

	// set up 'ready' channels to indicate if a goroutine has terminated
	collectMetricsReady := make(chan bool, 1)
	sendMetricsReady := make(chan bool, 1)
	getSettingsReady := make(chan bool, 1)
	settingsTimeoutCheckReady := make(chan bool, 1)
	collectMetricsReady <- true
	sendMetricsReady <- true
	getSettingsReady <- true
	settingsTimeoutCheckReady <- true

	for {
		// Exit if the reporter's done channel is closed.
		select {
		case <-r.done:
			return
		default:
		}
		select {
		case <-collectMetricsTicker.C: // collect and send metrics
			// set up ticker for next round
			collectMetricsTicker.Reset(r.collectMetricsNextInterval())
			select {
			case <-collectMetricsReady:
				// only kick off a new goroutine if the previous one has terminated
				go r.collectMetrics(collectMetricsReady, sendMetricsReady)
			default:
			}
		case <-getSettingsTicker.C: // get settings from collector
			// set up ticker for next round
			getSettingsTicker.Reset(time.Duration(r.getSettingsInterval) * time.Second)
			select {
			case <-getSettingsReady:
				// only kick off a new goroutine if the previous one has terminated
				go r.getSettings(getSettingsReady)
			default:
			}
		case <-settingsTimeoutCheckTicker.C: // check for timed out settings
			// set up ticker for next round
			settingsTimeoutCheckTicker.Reset(time.Duration(r.settingsTimeoutCheckInterval) * time.Second)
			select {
			case <-settingsTimeoutCheckReady:
				// only kick off a new goroutine if the previous one has terminated
				go r.checkSettingsTimeout(settingsTimeoutCheckReady)
			default:
			}
		case <-r.eventConnection.pingTicker.C: // ping on event connection (keep alive)
			// set up ticker for next round
			r.eventConnection.resetPing()
			go func() {
				if r.eventConnection.ping(r.done, r.serviceKey) == errInvalidServiceKey {
					r.Shutdown()
				}
			}()
		case <-r.metricConnection.pingTicker.C: // ping on metrics connection (keep alive)
			// set up ticker for next round
			r.metricConnection.resetPing()
			go func() {
				if r.metricConnection.ping(r.done, r.serviceKey) == errInvalidServiceKey {
					r.Shutdown()
				}
			}()
		case <-r.done:
			return
		}
	}
}

type Backoff func(retries int, wait func(d time.Duration)) error

// DefaultBackoff calls the wait function to sleep for a certain time based on
// the retries value. It returns immediately if the retries exceeds a threshold.
func DefaultBackoff(retries int, wait func(d time.Duration)) error {
	if retries > grpcMaxRetries {
		return ErrMaxRetriesExceeded
	}
	delay := int(grpcRetryDelayInitial * math.Pow(grpcRetryDelayMultiplier, float64(retries-1)))
	if delay > grpcRetryDelayMax*1000 {
		delay = grpcRetryDelayMax * 1000
	}

	wait(time.Duration(delay) * time.Millisecond)
	return nil
}

// ================================ Event Handling ====================================

// prepares the given event and puts it on the channel so it can be consumed by the
// eventSender() goroutine
// ctx		oboe context
// e		event to be put on the channel
//
// returns	error if something goes wrong during preparation or if channel is full
func (r *grpcReporter) reportEvent(ctx *oboeContext, e *event) error {
	if r.Closed() {
		return ErrReporterIsClosed
	}
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	select {
	case r.eventMessages <- (*e).bbuf.GetBuf():
		atomic.AddInt64(&r.eventConnection.queueStats.totalEvents, int64(1))
		return nil
	default:
		atomic.AddInt64(&r.eventConnection.queueStats.numOverflowed, int64(1))
		return errors.New("event message queue is full")
	}
}

// eventSender is a long-running goroutine that listens on the events message
// channel, collects all messages on that channel and attempts to send them to
// the collector using the gRPC method PostEvents()
func (r *grpcReporter) eventSender() {
	defer log.Warning("eventSender goroutine exiting.")

	batches := make(chan [][]byte)
	token := r.eventBatchSender(batches)
	opts := config.ReporterOpts()

	// This event bucket is drainable either after it reaches HWM, or the flush
	// interval has passed.
	evtBucket := NewBytesBucket(r.eventMessages,
		WithHWM(int(opts.GetEventBatchSize()*1024)),
		WithIntervalGetter(opts.GetEventFlushInterval))

	for {
		select {
		// Check if the agent is required to quit.
		case <-r.done:
			return
		default:
		}

		// Pour as much water as we can into the bucket, until it's full or
		// no more water can be got from the source. It's not blocking here.
		evtBucket.PourIn()

		// The events can only be pushed into the channel when the bucket
		// is drainable (either full or timeout) and we've got the token
		// to push events.
		if evtBucket.Drainable() {
			select {
			case <-token:
				w := evtBucket.Watermark()
				batches <- evtBucket.Drain()
				log.Debugf("Pushed %d bytes to event sender.", w)
			default:
			}
		}

		// Don't consume too much CPU with noop
		time.Sleep(time.Millisecond * 100)
	}
}

func (r *grpcReporter) eventBatchSender(batches chan [][]byte) chan struct{} {
	token := make(chan struct{})
	go func() {
		r.eventRetrySender(batches, token)
	}()
	return token
}

func (r *grpcReporter) eventRetrySender(
	batches <-chan [][]byte,
	token chan<- struct{},
) {
	defer log.Warning("eventRetrySender goroutine exiting.")

	// Push the token to the queue side to kick it off
	token <- struct{}{}

	for {
		var messages [][]byte
		// this will block until a message arrives or the reporter is closed

		select {
		case b := <-batches:
			messages = b
		case <-r.done:
			return
		}

		method := newPostEventsMethod(r.serviceKey, messages)
		err := r.eventConnection.InvokeRPC(r.done, method)

		switch err {
		case errInvalidServiceKey:
			r.Shutdown()
		case nil, errGiveUpAfterRetries, errTooManyRedirections:
			token <- struct{}{}
		default:
			log.Infof("eventRetrySender: %s", err)
		}
	}
}

// ================================ Metrics Handling ====================================

// calculates the interval from now until the next time we need to collect metrics
//
// returns	the interval (nanoseconds)
func (r *grpcReporter) collectMetricsNextInterval() time.Duration {
	r.collectMetricIntervalLock.RLock()
	interval := r.collectMetricInterval - (time.Now().Second() % r.collectMetricInterval)
	r.collectMetricIntervalLock.RUnlock()
	return time.Duration(interval) * time.Second
}

// collects the current metrics, puts them on the channel, and kicks off sendMetrics()
// collectReady	a 'ready' channel to indicate if this routine has terminated
// sendReady	another 'ready' channel to indicate if the sendMetrics() goroutine has terminated
func (r *grpcReporter) collectMetrics(collectReady chan bool, sendReady chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { collectReady <- true }()

	// access collectMetricInterval protected since it can be updated in updateSettings()
	r.collectMetricIntervalLock.RLock()
	interval := r.collectMetricInterval
	r.collectMetricIntervalLock.RUnlock()

	// generate a new metrics message
	message := generateMetricsMessage(interval, r.eventConnection.queueStats)
	// 	printBson(message)

	select {
	// put metrics message onto the channel
	case r.metricMessages <- message:
	default:
	}

	select {
	case <-sendReady:
		// only kick off a new goroutine if the previous one has terminated
		go r.sendMetrics(sendReady)
	default:
	}
}

// listens on the metrics message channel, collects all messages on that channel and
// attempts to send them to the collector using the GRPC method PostMetrics()
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) sendMetrics(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	var messages [][]byte

	done := false
	for !done {
		select {
		case m := <-r.metricMessages:
			messages = append(messages, m)
		default:
			done = true
		}
	}

	// no messages on the channel so nothing to send, return
	if len(messages) == 0 {
		return
	}

	method := newPostMetricsMethod(r.serviceKey, messages)

	err := r.metricConnection.InvokeRPC(r.done, method)
	switch err {
	case errInvalidServiceKey:
		r.Shutdown()
	case nil:
	default:
		log.Infof("sendMetrics: %s", err)
	}
}

// ================================ Settings Handling ====================================

// retrieves the settings from the collector
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) getSettings(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	method := newGetSettingsMethod(r.serviceKey)
	err := r.metricConnection.InvokeRPC(r.done, method)

	switch err {
	case errInvalidServiceKey:
		r.Shutdown()
	case nil:
		r.updateSettings(method.Resp)
	default:
		log.Infof("getSettings: %s", err)
	}
}

// updates the existing settings with the newly received
// settings	new settings
func (r *grpcReporter) updateSettings(settings *collector.SettingsResult) {
	for _, s := range settings.GetSettings() {
		updateSetting(int32(s.Type), string(s.Layer), s.Flags, s.Value, s.Ttl, &s.Arguments)

		// update MetricsFlushInterval
		r.collectMetricIntervalLock.Lock()
		if interval, ok := s.Arguments["MetricsFlushInterval"]; ok {
			l := len(interval)
			if l == 4 {
				r.collectMetricInterval = int(binary.LittleEndian.Uint32(interval))
			} else {
				log.Warningf("Bad MetricsFlushInterval size: %s", l)
			}
		} else {
			r.collectMetricInterval = grpcMetricIntervalDefault
		}
		r.collectMetricIntervalLock.Unlock()

		// update events flush interval
		if interval, ok := s.Arguments["EventsFlushInterval"]; ok {
			l := len(interval)
			if l == 4 {
				newInterval := int64(binary.LittleEndian.Uint32(interval))
				config.ReporterOpts().SetEventFlushInterval(newInterval)
			} else {
				log.Warningf("Bad EventsFlushInterval size: %s", l)
			}
		}

		// update MaxTransactions
		capacity := metricsTransactionsMaxDefault
		if max, ok := s.Arguments["MaxTransactions"]; ok {
			capacity = int(binary.LittleEndian.Uint32(max))
		}
		mTransMap.SetCap(capacity)
	}

	if hasDefaultSetting() {
		r.setReady()
	}
}

// delete settings that have timed out according to their TTL
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) checkSettingsTimeout(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	OboeCheckSettingsTimeout()
}

// ========================= Status Message Handling =============================

// prepares the given event and puts it on the channel so it can be consumed by the
// statusSender() goroutine
// ctx		oboe context
// e		event to be put on the channel
//
// returns	error if something goes wrong during preparation or if channel is full
func (r *grpcReporter) reportStatus(ctx *oboeContext, e *event) error {
	if r.Closed() {
		return ErrReporterIsClosed
	}
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	select {
	case r.statusMessages <- (*e).bbuf.GetBuf():
		return nil
	default:
		return errors.New("status message queue is full")
	}
}

// long-running goroutine that listens on the status message channel, collects all messages
// on that channel and attempts to send them to the collector using the GRPC method PostStatus()
func (r *grpcReporter) statusSender() {
	defer log.Warning("statusSender goroutine exiting.")

	for {
		var messages [][]byte

		select {
		// this will block until a message arrives
		case e := <-r.statusMessages:
			messages = append(messages, e)
		case <-r.done: // Exit if the reporter's done channel is closed.
			return
		}
		// one message detected, see if there are more and get them all!
		done := false
		for !done {
			select {
			case e := <-r.statusMessages:
				messages = append(messages, e)
			default:
				done = true
			}
		}
		method := newPostStatusMethod(r.serviceKey, messages)
		err := r.metricConnection.InvokeRPC(r.done, method)

		switch err {
		case errInvalidServiceKey:
			r.Shutdown()
		case nil:
		default:
			log.Infof("statusSender: %s", err)
		}
	}
}

// ========================= Span Message Handling =============================

// puts the given span messages on the channel so it can be consumed by the spanMessageAggregator()
// goroutine
// span		span message to be put on the channel
//
// returns	error if channel is full
func (r *grpcReporter) reportSpan(span SpanMessage) error {
	if r.Closed() {
		return ErrReporterIsClosed
	}
	select {
	case r.spanMessages <- span:
		return nil
	default:
		return errors.New("span message queue is full")
	}
}

// long-running goroutine that listens on the span message channel and processes (aggregates)
// incoming span messages
func (r *grpcReporter) spanMessageAggregator() {
	defer log.Warning("spanMessageAggregator goroutine exiting.")
	for {
		select {
		case span := <-r.spanMessages:
			span.process()
		case <-r.done:
			return
		}
	}
}

// ========================= Ping Handling =============================

// reset keep alive timer on a given GRPC connection
func (c *grpcConnection) resetPing() {
	if c.pingTicker == nil {
		return
	}
	c.pingTickerLock.Lock()
	// TODO: Reset may run into a race condition
	c.pingTicker.Reset(time.Duration(grpcPingIntervalDefault) * time.Second)
	c.pingTickerLock.Unlock()
}

// send a keep alive (ping) request on a given GRPC connection
func (c *grpcConnection) ping(exit chan struct{}, key string) error {
	method := newPingMethod(key, c.name)
	return c.InvokeRPC(exit, method)
}

// possible errors while issuing an RPC call
var (
	// The collector notifies that the service key of this reporter is invalid.
	// The reporter should be closed in this case.
	errInvalidServiceKey = errors.New("invalid service key")

	// Only a certain amount of retries are allowed. The message will be dropped
	// if the number of retries exceeds this number.
	errGiveUpAfterRetries = errors.New("give up after retries")

	// The maximum number of redirections has reached and the message will be
	// dropped.
	errTooManyRedirections = errors.New("too many redirections")

	errReporterExiting = errors.New("reporter is exiting")

	errShouldNotHappen = errors.New("this should not happen")

	errNoRetryOnErr = errors.New("method requires no retry")
)

// InvokeRPC makes an RPC call and returns an error if something is broken and
// cannot be handled by itself, e.g., the collector's response indicates the
// service key is invalid. It maintains the connection and does the retries
// automatically and transparently. It may give up after a certain times of
// retries, so it is a best-effort service only.
//
// When an error is returned, it usually means a fatal error and the reporter
// may be shutdown.
func (c *grpcConnection) InvokeRPC(exit chan struct{}, m Method) error {
	c.queueStats.setQueueLargest(m.MessageLen())

	// counter for redirects so we know when the limit has been reached
	redirects := 0

	// Number of gRPC errors encountered
	failsNum := 0
	// Number of retries, including gRPC errors and collector errors
	retriesNum := 0

	for {
		// Fail-fast in case the reporter has been closed, avoid retrying in
		// this case.
		select {
		case <-exit:
			return errReporterExiting
		default:
		}

		var err = errors.New("connection is stale")
		// Protect the call to the client object or we could run into problems
		// if another goroutine is messing with it at the same time, e.g. doing
		// a redirection.
		c.lock.RLock()
		if c.isActive() {
			ctx, cancel := context.WithTimeout(context.Background(), grpcCtxTimeout)
			err = m.Call(ctx, c.client)

			code := status.Code(err)
			if code == codes.DeadlineExceeded ||
				code == codes.Canceled {
				log.Infof("[%s] Connection becomes stale: %v", m, err)
				err = errors.Wrap(err, "connection is stale")
				c.setActive(false)
			}
			cancel()
		}
		c.lock.RUnlock()

		// we sent something, or at least tried to, so we're not idle - reset
		// the keepalive timer
		c.resetPing()

		if err != nil {
			// gRPC handles the reconnection automatically.
			failsNum++
			if failsNum == grpcRetryLogThreshold {
				log.Warningf("[%s] invocation error: %v", m, err)
			} else {
				log.Debugf("[%s] (%v) invocation error: %v", m, failsNum, err)
			}
		} else {
			if failsNum >= grpcRetryLogThreshold {
				log.Warningf("[%s] error recovered.", m)
			}
			failsNum = 0

			// server responded, check the result code and perform actions accordingly
			switch result := m.ResultCode(); result {
			case collector.ResultCode_OK:
				log.Infof("[%s] sent %d data chunks", m, m.MessageLen())
				atomic.AddInt64(&c.queueStats.numSent, m.MessageLen())
				return nil

			case collector.ResultCode_TRY_LATER:
				log.Infof("[%s] server responded: Try later", m)
				atomic.AddInt64(&c.queueStats.numFailed, m.MessageLen())
			case collector.ResultCode_LIMIT_EXCEEDED:
				log.Infof("[%s] server responded: Limit exceeded", m)
				atomic.AddInt64(&c.queueStats.numFailed, m.MessageLen())
			case collector.ResultCode_INVALID_API_KEY:
				log.Errorf("[%s] server responded: Invalid API key.", m)
				return errInvalidServiceKey
			case collector.ResultCode_REDIRECT:
				if redirects > grpcRedirectMax {
					log.Errorf("[%s] max redirects of %v exceeded", m, grpcRedirectMax)
					return errTooManyRedirections
				} else {
					log.Warningf("[%s] channel is redirecting to %s", m, m.Arg())

					c.setAddress(m.Arg())
					// a proper redirect shouldn't cause delays
					retriesNum = 0
					redirects++
				}
			default:
				log.Infof("[%s] unknown server response: %v", m, result)
			}
		}

		if !c.isActive() {
			c.reconnect()
		}

		if !m.RetryOnErr() {
			return errNoRetryOnErr
		}

		retriesNum++
		err = c.backoff(retriesNum, func(d time.Duration) {
			time.Sleep(d)
		})
		if err != nil {
			return errGiveUpAfterRetries
		}
	}
	return errShouldNotHappen
}

func newHostID(id host.ID) *collector.HostID {
	gid := &collector.HostID{}

	gid.Hostname = id.Hostname()

	// DEPRECATED: IP addresses and UUID are not part of the host ID anymore
	// but kept for a while due to backward-compatibility.
	gid.IpAddresses = host.IPAddresses()
	gid.Uuid = ""

	gid.Pid = int32(id.Pid())
	gid.Ec2InstanceID = id.EC2Id()
	gid.Ec2AvailabilityZone = id.EC2Zone()
	gid.DockerContainerID = id.ContainerId()
	gid.MacAddresses = id.MAC()
	gid.HerokuDynoID = id.HerokuID()

	return gid
}

// buildIdentity builds the HostID struct from current host metadata
func buildIdentity() *collector.HostID {
	return newHostID(host.CurrentID())
}

// buildBestEffortIdentity builds the HostID with the best effort
func buildBestEffortIdentity() *collector.HostID {
	hid := newHostID(host.BestEffortCurrentID())
	hid.Hostname = host.Hostname()
	return hid
}

// Dialer has a method Dial which accepts a grpcConnection object as the
// argument and returns a ClientConn object.
type Dialer interface {
	Dial(grpcConnection) (*grpc.ClientConn, error)
}

// DefaultDialer implements the Dialer interface to provide the default dialing
// method.
type DefaultDialer struct{}

// Dial issues the connection to the remote address with attributes provided by
// the grpcConnection.
func (d *DefaultDialer) Dial(c grpcConnection) (*grpc.ClientConn, error) {
	certPool := x509.NewCertPool()

	if ok := certPool.AppendCertsFromPEM(c.certificate); !ok {
		return nil, errors.New("unable to append the certificate to pool")
	}

	// trim port from server name used for TLS verification
	serverName := c.address
	if s := strings.Split(c.address, ":"); len(s) > 0 {
		serverName = s[0]
	}

	tlsConfig := &tls.Config{
		ServerName:         serverName,
		RootCAs:            certPool,
		InsecureSkipVerify: c.insecureSkipVerify,
	}
	// turn off server certificate verification for Go < 1.8
	if !utils.IsHigherOrEqualGoVersion("go1.8") {
		tlsConfig.InsecureSkipVerify = true
	}
	creds := credentials.NewTLS(tlsConfig)

	return grpc.Dial(c.address, grpc.WithTransportCredentials(creds))
}
