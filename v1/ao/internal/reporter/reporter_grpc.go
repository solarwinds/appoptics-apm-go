// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	collector "github.com/solarwindscloud/apm-proto/go/collectorpb"
	"io"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/pkg/errors"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"

	"context"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/host"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	uatomic "go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	grpcReporterVersion = "2"

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
	grpcMetricIntervalDefault               = 60               // default metrics flush interval in seconds
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
	queueStats     *metrics.EventQueueStats       // queue stats (reset on each metrics report cycle)

	proxy            string
	proxyTLSCertPath string

	// atomicActive indicates if the underlying connection is active. It should
	// be reconnected or redirected to a new address in case of inactive. The
	// value 0 represents false and a value other than 0 (usually 1) means true
	atomicActive int32

	// the backoff function
	backoff Backoff
	Dialer

	// This channel is closed after flushing the metrics.
	flushed     chan struct{}
	flushedOnce sync.Once
	maxReqBytes int64 // the maximum size for an RPC request body
}

// GrpcConnOpt defines the function type that sets an option of the grpcConnection
type GrpcConnOpt func(c *grpcConnection)

// WithCert returns a function that sets the certificate
func WithCert(cert []byte) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.certificate = cert
	}
}

// WithProxy assign the proxy url to the gRPC connection
func WithProxy(proxy string) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.proxy = proxy
	}
}

// WithProxyCertPath assigns the proxy TLS certificate path to the gRPC connection
func WithProxyCertPath(certPath string) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.proxyTLSCertPath = certPath
	}
}

// WithMaxReqBytes sets the maximum size of an RPC request
func WithMaxReqBytes(size int64) GrpcConnOpt {
	return func(c *grpcConnection) {
		c.maxReqBytes = size
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
		name:        name,
		client:      nil,
		connection:  nil,
		address:     target,
		certificate: []byte(grpcCertDefault),
		pingTicker:  time.NewTimer(time.Duration(grpcPingIntervalDefault) * time.Second),
		queueStats:  &metrics.EventQueueStats{},
		backoff:     DefaultBackoff,
		Dialer:      &DefaultDialer{},
		flushed:     make(chan struct{}),
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
	conn                         *grpcConnection // used for all RPC calls
	collectMetricInterval        int32           // metrics flush interval in seconds
	getSettingsInterval          int             // settings retrieval interval in seconds
	settingsTimeoutCheckInterval int             // check interval for timed out settings in seconds

	serviceKey *uatomic.String // service key

	eventMessages  chan []byte              // channel for event messages (sent from agent)
	spanMessages   chan metrics.SpanMessage // channel for span messages (sent from agent)
	statusMessages chan []byte              // channel for status messages (sent from agent)

	httpMetrics   *metrics.Measurements
	customMetrics *metrics.Measurements

	// The reporter is considered ready if there is a valid default setting for sampling.
	// It should be accessed atomically.
	ready int32
	// The condition variable to notify those who are waiting for the reporter becomes ready
	cond *sync.Cond

	// The reporter doesn't have a explicit field to record its state. This channel is used to notify all the
	// long-running goroutines to stop themselves. All the goroutines will check this channel and close if the
	// channel is closed.
	// Don't send data into this channel, just close it by calling Shutdown().
	done       chan struct{}
	doneClosed sync.Once
	// The flag to indicate gracefully stopping the reporter. It should be accessed atomically.
	// A (default) zero value means shutdown abruptly.
	gracefully int32
}

// gRPC reporter errors
var (
	ErrShutdownClosedReporter = errors.New("trying to shutdown a closed reporter")
	ErrShutdownTimeout        = errors.New("Shutdown timeout")
	ErrReporterIsClosed       = errors.New("the reporter is closed")
)

func getProxy() string {
	return config.GetProxy()
}

func getProxyCertPath() string {
	return config.GetProxyCertPath()
}

// initializes a new GRPC reporter from scratch (called once on program startup)
//
// returns	GRPC reporter object
func newGRPCReporter() reporter {
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

	opts = append(opts, WithMaxReqBytes(config.ReporterOpts().GetMaxReqBytes()))

	if proxy := getProxy(); proxy != "" {
		opts = append(opts, WithProxy(proxy))
		opts = append(opts, WithProxyCertPath(getProxyCertPath()))
	}

	// create connection object for events client and metrics client
	grpcConn, err1 := newGrpcConnection("AO gRPC channel", addr, opts...)
	if err1 != nil {
		log.Errorf("Failed to initialize gRPC reporter %v: %v", addr, err1)
		return &nullReporter{}
	}

	r := &grpcReporter{
		conn: grpcConn,

		collectMetricInterval:        grpcMetricIntervalDefault,
		getSettingsInterval:          grpcGetSettingsIntervalDefault,
		settingsTimeoutCheckInterval: grpcSettingsTimeoutCheckIntervalDefault,

		serviceKey: uatomic.NewString(config.GetServiceKey()),

		eventMessages:  make(chan []byte, 10000),
		spanMessages:   make(chan metrics.SpanMessage, 10000),
		statusMessages: make(chan []byte, 100),
		httpMetrics:    metrics.NewMeasurements(false, grpcMetricIntervalDefault, 200),
		customMetrics:  metrics.NewMeasurements(true, grpcMetricIntervalDefault, 500), // TODO configurable

		cond: sync.NewCond(&sync.Mutex{}),
		done: make(chan struct{}),
	}

	r.start()

	log.Warningf("The reporter (%v, v%v, go%v) is initialized. Waiting for the dynamic settings.",
		r.done, utils.Version(), utils.GoVersion())
	return r
}

func (r *grpcReporter) Flush() error {
	return nil
}

func (r *grpcReporter) SetServiceKey(key string) {
	r.serviceKey.Store(key)
}

func (r *grpcReporter) isGracefully() bool {
	return atomic.LoadInt32(&r.gracefully) == 1
}

func (r *grpcReporter) setGracefully(flag bool) {
	var i int32
	if flag {
		i = 1
	}
	atomic.StoreInt32(&r.gracefully, i)
}

func (r *grpcReporter) start() {
	// start up the host observer
	host.Start()
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

// ShutdownNow stops the reporter immediately.
func (r *grpcReporter) ShutdownNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	return r.Shutdown(ctx)
}

// Shutdown closes the reporter by close the `done` channel. All long-running goroutines
// monitor the channel `done` in the reporter and close themselves when the channel is closed.
func (r *grpcReporter) Shutdown(ctx context.Context) error {
	var err error

	select {
	case <-r.done:
		err = ErrShutdownClosedReporter
	default:
		r.doneClosed.Do(func() {
			log.Warningf("Shutting down the reporter: %v", r.done)

			var g bool
			if d, ddlSet := ctx.Deadline(); ddlSet {
				g = d.Sub(time.Now()) > 0
			} else {
				g = true
			}
			r.setGracefully(g)

			close(r.done)

			select {
			case <-r.flushed():
			case <-ctx.Done():
				err = ErrShutdownTimeout
			}

			r.closeConns()
			r.setReady(false)
			host.Stop()
			log.Warning("AppOptics APM agent is stopped.")
		})
	}
	return err
}

func (r *grpcReporter) flushed() chan struct{} {
	c := make(chan struct{})
	go func(o chan struct{}) {
		chs := []chan struct{}{
			r.conn.getFlushedChan(),
		}
		for _, ch := range chs {
			<-ch
		}
		close(c)
	}(c)
	return c
}

// closeConns closes all the gRPC connections of a reporter
func (r *grpcReporter) closeConns() {
	r.conn.Close()
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

func (r *grpcReporter) setReady(ready bool) {
	var s int32
	if ready {
		s = 1
	}
	atomic.StoreInt32(&r.ready, s)
}

func (r *grpcReporter) isReady() bool {
	return atomic.LoadInt32(&r.ready) == 1
}

// WaitForReady waits until the reporter becomes ready or the context is canceled.
//
// The reporter is still considered `not ready` if (in rare cases) the default
// setting is retrieved from the collector but expires after the TTL, and no new
// setting is fetched.
func (r *grpcReporter) WaitForReady(ctx context.Context) bool {
	if r.isReady() {
		return true
	}

	ready := make(chan struct{})
	var e int32

	go func(ch chan struct{}, exit *int32) {
		r.cond.L.Lock()
		for !r.isReady() && (atomic.LoadInt32(exit) != 1) {
			r.cond.Wait()
		}
		r.cond.L.Unlock()
		close(ch)
	}(ready, &e)

	select {
	case <-ready:
		return true
	case <-ctx.Done():
		atomic.StoreInt32(&e, 1)
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
		log.Debugf("[%s] Someone else has done the redirection.", c.name)
		return nil
	}
	// create a new connection object for this client
	conn, err := c.Dial(DialParams{
		Certificate:   c.certificate,
		Address:       c.address,
		Proxy:         c.proxy,
		ProxyCertPath: c.proxyTLSCertPath,
	})
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
	defer log.Info("periodicTasks goroutine exiting.")

	// set up tickers
	collectMetricsTicker := time.NewTimer(r.collectMetricsNextInterval())
	getSettingsTicker := time.NewTimer(0)
	settingsTimeoutCheckTicker := time.NewTimer(time.Duration(r.settingsTimeoutCheckInterval) * time.Second)

	defer func() {
		collectMetricsTicker.Stop()
		getSettingsTicker.Stop()
		settingsTimeoutCheckTicker.Stop()
		r.conn.pingTicker.Stop()
	}()

	// set up 'ready' channels to indicate if a goroutine has terminated
	collectMetricsReady := make(chan bool, 1)
	getSettingsReady := make(chan bool, 1)
	settingsTimeoutCheckReady := make(chan bool, 1)
	collectMetricsReady <- true
	getSettingsReady <- true
	settingsTimeoutCheckReady <- true

	for {
		// Exit if the reporter's done channel is closed.
		select {
		case <-r.done:
			if !r.isGracefully() {
				return
			}
		default:
		}

		select {
		case <-r.done:
			if !r.isGracefully() {
				return
			}
			select {
			case <-collectMetricsReady:
				r.collectMetrics(collectMetricsReady)
			default:
			}
			<-collectMetricsReady
			r.conn.setFlushed()
			return
		case <-collectMetricsTicker.C: // collect and send metrics
			// set up ticker for next round
			collectMetricsTicker.Reset(r.collectMetricsNextInterval())
			select {
			case <-collectMetricsReady:
				// only kick off a new goroutine if the previous one has terminated
				go r.collectMetrics(collectMetricsReady)
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
		case <-r.conn.pingTicker.C: // ping on event connection (keep alive)
			// set up ticker for next round
			r.conn.resetPing()
			go func() {
				if r.conn.ping(r.done, r.serviceKey.Load()) == errInvalidServiceKey {
					r.ShutdownNow()
				}
			}()
		}
	}
}

type Backoff func(retries int, wait func(d time.Duration)) error

// DefaultBackoff calls the wait function to sleep for a certain time based on
// the retries value. It returns immediately if the retries exceeds a threshold.
func DefaultBackoff(retries int, wait func(d time.Duration)) error {
	if retries > grpcMaxRetries {
		return errGiveUpAfterRetries
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
		r.conn.queueStats.TotalEventsAdd(int64(1))
		return nil
	default:
		r.conn.queueStats.NumOverflowedAdd(int64(1))
		return errors.New("event message queue is full")
	}
}

// eventSender is a long-running goroutine that listens on the events message
// channel, collects all messages on that channel and attempts to send them to
// the collector using the gRPC method PostEvents()
func (r *grpcReporter) eventSender() {
	batches := make(chan [][]byte, 10)
	defer func() {
		close(batches)
		log.Info("eventSender goroutine exiting.")
	}()

	go r.eventBatchSender(batches)

	opts := config.ReporterOpts()
	hwm := int(opts.GetMaxReqBytes())
	if hwm <= 0 {
		log.Warningf("The event sender is disabled by setting hwm=%d", hwm)
		hwm = 0
	}

	// This event bucket is drainable either after it reaches HWM, or the flush
	// interval has passed.
	evtBucket := NewBytesBucket(r.eventMessages,
		WithHWM(hwm),
		WithGracefulShutdown(r.isGracefully()),
		WithClosingIndicator(r.done),
		WithIntervalGetter(func() time.Duration {
			return time.Second * time.Duration(opts.GetEventFlushInterval())
		}),
	)

	for {
		// Pour as much water as we can into the bucket. It blocks until it's
		// full or timeout.
		evtBucket.PourIn()
		// The events can only be pushed into the channel when the bucket
		// is drainable (either full or timeout) and we've got the token
		// to push events.
		//
		// If the token is holding by eventRetrySender, it usually means the
		// events sending is too slow (or the events are generated too fast).
		// We have to wait in this case.
		//
		// If the reporter is closing, we may have the last chance to send all
		// the queued events.
		if evtBucket.Full() {
			c := evtBucket.Count()
			dropped := evtBucket.DroppedCount()
			if dropped != 0 {
				log.Infof("Pushed %d events to the sender, dropped %d oversize events.", c, dropped)
			} else {
				log.Debugf("Pushed %d events to the sender.", c)
			}

			batches <- evtBucket.Drain()
		}

		select {
		// Check if the agent is required to quit.
		case <-r.done:
			return
		default:
		}

	}
}

func (r *grpcReporter) eventBatchSender(batches <-chan [][]byte) {
	defer func() {
		r.conn.setFlushed()
		log.Info("eventBatchSender goroutine exiting.")
	}()

	var closing bool
	var messages [][]byte

	for {
		// this will block until a message arrives or the reporter is closed
		select {
		case messages = <-batches:
			if len(messages) == 0 {
				batches = nil
			}
		case <-r.done:
			select {
			case messages = <-batches:
			default:
			}
			if !r.isGracefully() {
				return
			}
			closing = true
		}

		if len(messages) != 0 {
			method := newPostEventsMethod(r.serviceKey.Load(), messages)
			err := r.conn.InvokeRPC(r.done, method)

			switch err {
			case errInvalidServiceKey:
				r.ShutdownNow()
			case nil:
				log.Info(method.CallSummary())
			default:
				log.Warningf("eventBatchSender: %s", err)
			}
		}

		if closing {
			return
		}
	}
}

// ================================ Metrics Handling ====================================

// calculates the interval from now until the next time we need to collect metrics
//
// returns	the interval (nanoseconds)
func (r *grpcReporter) collectMetricsNextInterval() time.Duration {
	i := int(atomic.LoadInt32(&r.collectMetricInterval))
	interval := i - (time.Now().Second() % i)
	return time.Duration(interval) * time.Second
}

// collects the current metrics, puts them on the channel, and kicks off sendMetrics()
// collectReady	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) collectMetrics(collectReady chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { collectReady <- true }()

	i := atomic.LoadInt32(&r.collectMetricInterval)

	var messages [][]byte
	// generate a new metrics message
	builtin := metrics.BuildBuiltinMetricsMessage(r.httpMetrics.CopyAndReset(i),
		r.conn.queueStats.CopyAndReset(), FlushRateCounts(), config.GetRuntimeMetrics())
	if builtin != nil {
		messages = append(messages, builtin)
	}

	custom := metrics.BuildMessage(r.customMetrics.CopyAndReset(i), false)
	if custom != nil {
		messages = append(messages, custom)
	}

	r.sendMetrics(messages)
}

// listens on the metrics message channel, collects all messages on that channel and
// attempts to send them to the collector using the GRPC method PostMetrics()
func (r *grpcReporter) sendMetrics(msgs [][]byte) {
	// no messages on the channel so nothing to send, return
	if len(msgs) == 0 {
		return
	}

	method := newPostMetricsMethod(r.serviceKey.Load(), msgs)

	err := r.conn.InvokeRPC(r.done, method)
	switch err {
	case errInvalidServiceKey:
		r.ShutdownNow()
	case nil:
		log.Info(method.CallSummary())
	default:
		log.Warningf("sendMetrics: %s", err)
	}
}

// CustomSummaryMetric submits a summary type measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func (r *grpcReporter) CustomSummaryMetric(name string, value float64, opts metrics.MetricOptions) error {
	return r.customMetrics.Summary(name, value, opts)
}

// CustomIncrementMetric submits a incremental measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func (r *grpcReporter) CustomIncrementMetric(name string, opts metrics.MetricOptions) error {
	return r.customMetrics.Increment(name, opts)
}

// ================================ Settings Handling ====================================

// retrieves the settings from the collector
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) getSettings(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	method := newGetSettingsMethod(r.serviceKey.Load())
	err := r.conn.InvokeRPC(r.done, method)

	switch err {
	case errInvalidServiceKey:
		r.ShutdownNow()
	case nil:
		logger := log.Info
		if method.Resp.Warning != "" {
			logger = log.Warning
		}
		logger(method.CallSummary())
		r.updateSettings(method.Resp)
	default:
		log.Infof("getSettings: %s", err)
	}
}

// updates the existing settings with the newly received
// settings	new settings
func (r *grpcReporter) updateSettings(settings *collector.SettingsResult) {
	for _, s := range settings.GetSettings() {
		updateSetting(int32(s.Type), string(s.Layer), s.Flags, s.Value, s.Ttl, s.Arguments)

		// update MetricsFlushInterval
		mi := parseInt32(s.Arguments, kvMetricsFlushInterval, r.collectMetricInterval)
		atomic.StoreInt32(&r.collectMetricInterval, mi)

		// update events flush interval
		o := config.ReporterOpts()
		ei := parseInt32(s.Arguments, kvEventsFlushInterval, int32(o.GetEventFlushInterval()))
		o.SetEventFlushInterval(int64(ei))

		// update MaxTransactions
		mt := parseInt32(s.Arguments, kvMaxTransactions, r.httpMetrics.Cap())
		r.httpMetrics.SetCap(mt)

		maxCustomMetrics := parseInt32(s.Arguments, kvMaxCustomMetrics, r.httpMetrics.Cap())
		r.customMetrics.SetCap(maxCustomMetrics)
	}

	if !r.isReady() && hasDefaultSetting() {
		r.cond.L.Lock()
		r.setReady(true)
		log.Warningf("Got dynamic settings. The AppOptics APM agent (%v) is ready.", r.done)
		r.cond.Broadcast()
		r.cond.L.Unlock()
	}
}

// delete settings that have timed out according to their TTL
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) checkSettingsTimeout(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	OboeCheckSettingsTimeout()
	if r.isReady() && !hasDefaultSetting() {
		log.Warningf("Sampling setting expired. AppOptics agent (%v) is not working.", r.done)
		r.setReady(false)
	}
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
	defer log.Info("statusSender goroutine exiting.")

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
		method := newPostStatusMethod(r.serviceKey.Load(), messages)
		err := r.conn.InvokeRPC(r.done, method)

		switch err {
		case errInvalidServiceKey:
			r.ShutdownNow()
		case nil:
			log.Info(method.CallSummary())
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
func (r *grpcReporter) reportSpan(span metrics.SpanMessage) error {
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
	defer log.Info("spanMessageAggregator goroutine exiting.")
	for {
		select {
		case span := <-r.spanMessages:
			span.Process(r.httpMetrics)
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
	err := c.InvokeRPC(exit, method)
	log.Debug(method.CallSummary())
	return err
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

	// The destination returned by the collector is not valid.
	errInvalidRedirectTarget = errors.New("redirection target is empty.")

	// the operation or loop cannot continue as the reporter is exiting.
	errReporterExiting = errors.New("reporter is exiting")

	// something might be wrong if we run into this error.
	errShouldNotHappen = errors.New("this should not happen")

	// errNoRetryOnErr means this RPC call method doesn't need retry, e.g., the
	// Ping method.
	errNoRetryOnErr = errors.New("method requires no retry")

	// errConnStale means the connection is broken. This usually happens
	// when an RPC call is timeout.
	errConnStale     = errors.New("connection is stale")
	errRequestTooBig = errors.New("RPC request is too big")
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
	c.queueStats.SetQueueLargest(m.MessageLen())

	// counter for redirects so we know when the limit has been reached
	redirects := 0
	// Number of gRPC errors encountered
	failsNum := 0
	// Number of retries, including gRPC errors and collector errors
	retriesNum := 0

	printRPCMsg(m)

	for {
		// Fail-fast in case the reporter has been closed, avoid retrying in
		// this case.
		select {
		case <-exit:
			if c.isFlushed() {
				return errReporterExiting
			}
		default:
		}
		var err = errConnStale
		// Protect the call to the client object or we could run into problems
		// if another goroutine is messing with it at the same time, e.g. doing
		// a redirection.
		c.lock.RLock()
		if c.isActive() {
			ctx, cancel := context.WithTimeout(context.Background(), grpcCtxTimeout)
			if m.RequestSize() > c.maxReqBytes {
				v := fmt.Sprintf("%d|%d", m.RequestSize(), c.maxReqBytes)
				err = errors.Wrap(errRequestTooBig, v)
			} else {
				if m.ServiceKey() != "" {
					err = m.Call(ctx, c.client)
				} else {
					err = nil
					log.Infof("%s is skipped as no service key is assigned.", m.String())
				}
			}

			code := status.Code(err)
			if code == codes.DeadlineExceeded ||
				code == codes.Canceled {
				log.Infof("[%s] Connection becomes stale: %v.", c.name, err)
				err = errConnStale
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
				log.Warningf("[%s] invocation error: %v.", m, err)
			} else {
				log.Debugf("[%s] (%v) invocation error: %v.", m, failsNum, err)
			}
		} else {
			if failsNum >= grpcRetryLogThreshold {
				log.Warningf("[%s] error recovered.", m)
			}
			failsNum = 0

			// server responded, check the result code and perform actions accordingly
			switch result, _ := m.ResultCode(); result {
			case collector.ResultCode_OK:
				c.queueStats.NumSentAdd(m.MessageLen())
				return nil

			case collector.ResultCode_TRY_LATER:
				log.Info(m.CallSummary())
				c.queueStats.NumFailedAdd(m.MessageLen())
			case collector.ResultCode_LIMIT_EXCEEDED:
				log.Info(m.CallSummary())
				c.queueStats.NumFailedAdd(m.MessageLen())
			case collector.ResultCode_INVALID_API_KEY:
				log.Error(m.CallSummary())
				return errInvalidServiceKey
			case collector.ResultCode_REDIRECT:
				log.Warning(m.CallSummary())
				redirects++

				if redirects > grpcRedirectMax {
					return errTooManyRedirections
				} else if m.Arg() != "" {
					c.setAddress(m.Arg())
					// a proper redirect shouldn't cause delays
					retriesNum = 0
				} else {
					log.Warning(errors.Wrap(errInvalidRedirectTarget, c.name))
				}
			default:
				log.Info(m.CallSummary())
			}
		}

		if !c.isActive() {
			c.reconnect()
		}

		if !m.RetryOnErr(err) {
			if err != nil {
				return errors.Wrap(errNoRetryOnErr, err.Error())
			} else {
				return errNoRetryOnErr
			}
		}

		retriesNum++
		err = c.backoff(retriesNum, func(d time.Duration) {
			time.Sleep(d)
		})
		if err != nil {
			return err
		}
	}
}

func (c *grpcConnection) setFlushed() {
	c.flushedOnce.Do(func() { close(c.flushed) })
}

func (c *grpcConnection) getFlushedChan() chan struct{} {
	return c.flushed
}

func (c *grpcConnection) isFlushed() bool {
	select {
	case <-c.flushed:
		return true
	default:
		return false
	}
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
	gid.HerokuDynoID = id.HerokuId()
	gid.AzAppServiceInstanceID = id.AzureAppInstId()
	gid.HostType = collector.HostType_PERSISTENT

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
	Dial(options DialParams) (*grpc.ClientConn, error)
}

type DialParams struct {
	Certificate   []byte
	Address       string
	Proxy         string
	ProxyCertPath string
}

// DefaultDialer implements the Dialer interface to provide the default dialing
// method.
type DefaultDialer struct{}

// Dial issues the connection to the remote address with attributes provided by
// the grpcConnection.
func (d *DefaultDialer) Dial(p DialParams) (*grpc.ClientConn, error) {
	certPool := x509.NewCertPool()

	if ok := certPool.AppendCertsFromPEM(p.Certificate); !ok {
		return nil, errors.New("unable to append the certificate to pool")
	}

	// trim port from server name used for TLS verification
	serverName := p.Address
	if s := strings.Split(p.Address, ":"); len(s) > 0 {
		serverName = s[0]
	}

	tlsConfig := &tls.Config{
		ServerName: serverName,
		RootCAs:    certPool,
	}

	creds := credentials.NewTLS(tlsConfig)

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	}

	if p.Proxy != "" {
		opts = append(opts, grpc.WithContextDialer(newGRPCProxyDialer(p)))
	}

	return grpc.Dial(p.Address, opts...)
}

func newGRPCProxyDialer(p DialParams) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (conn net.Conn, err error) {
		defer func() {
			if err != nil && conn != nil {
				conn.Close()
			}
		}()

		proxy, err := url.Parse(p.Proxy)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing the proxy url")
		}

		if proxy.Scheme == "https" {
			cert, err := ioutil.ReadFile(p.ProxyCertPath)
			if err != nil {
				return nil, errors.Wrap(err, "failed to load proxy cert")
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(cert)

			// No mutual TLS for now
			tlsConfig := tls.Config{RootCAs: caCertPool}
			conn, err = tls.Dial("tcp", proxy.Host, &tlsConfig)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial the https proxy")
			}
		} else if proxy.Scheme == "http" {
			conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", proxy.Host)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial the http proxy")
			}
		} else {
			return nil, fmt.Errorf("proxy scheme not supported: %s", proxy.Scheme)
		}

		return httpConnectHandshake(ctx, conn, addr, proxy)
	}
}

func httpConnectHandshake(ctx context.Context, conn net.Conn, server string, proxy *url.URL) (net.Conn, error) {
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: server},
		Header: map[string][]string{"User-Agent": {grpcUA}},
	}
	if t := proxy.User; t != nil {
		u := t.Username()
		p, _ := t.Password()
		req.Header.Add(proxyAuthHeader, "Basic "+basicAuth(u, p))
	}

	req = req.WithContext(ctx)
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write the HTTP request: %v", err)
	}

	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return nil, fmt.Errorf("reading server HTTP response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return nil, fmt.Errorf("failed to do connect handshake, status code: %s", resp.Status)
		}
		return nil, fmt.Errorf("failed to do connect handshake, response: %q", dump)
	}

	return &bufConn{Conn: conn, r: r}, nil
}

type bufConn struct {
	net.Conn
	r io.Reader
}

func (c *bufConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

const proxyAuthHeader = "Proxy-Authorization"
const grpcUA = "grpc-go/" + grpc.Version

func printRPCMsg(m Method) {
	if log.Level() > log.DEBUG {
		return
	}

	var str []string
	str = append(str, m.String())

	msgs := m.Message()
	for _, msg := range msgs {
		str = append(str, utils.SPrintBson(msg))
	}
	log.Debugf("%s", str)
}
