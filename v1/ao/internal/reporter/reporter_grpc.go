// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	grpcEventFlushBatchSizeDefault          = 2 * 1024 * 1024 // EventsBatchSize in bytes
	grpcMetricIntervalDefault               = 30              // default metrics flush interval in seconds
	grpcGetSettingsIntervalDefault          = 30              // default settings retrieval interval in seconds
	grpcSettingsTimeoutCheckIntervalDefault = 10              // default check interval for timed out settings in seconds
	grpcPingIntervalDefault                 = 20              // default interval for keep alive pings in seconds
	grpcRetryDelayInitial                   = 500             // initial connection/send retry delay in milliseconds
	grpcRetryDelayMultiplier                = 1.5             // backoff multiplier for unsuccessful retries
	grpcRetryDelayMax                       = 60              // max connection/send retry delay in seconds
	grpcRedirectMax                         = 20              // max allowed collector redirects
	grpcRetryLogThreshold                   = 10              // log prints after this number of retries (about 56.7s)
	grpcMaxRetries                          = 20              // The message will be dropped after this number of retries
)

type reporterChannel int

// a channel the reporter is listening on for messages from the agent
const (
	EVENTS reporterChannel = iota
	METRICS
)

// everything needed for a GRPC connection
type grpcConnection struct {
	client         collector.TraceCollectorClient // GRPC client instance
	connection     *grpc.ClientConn               // GRPC connection object
	address        string                         // collector address
	certificate    []byte                         // collector certificate
	serviceKey     string                         // service key
	pingTicker     *time.Timer                    // timer for keep alive pings in seconds
	pingTickerLock sync.Mutex                     // lock to ensure sequential access of pingTicker
	lock           sync.RWMutex                   // lock to ensure sequential access (in case of connection loss)
	queueStats     *eventQueueStats               // queue stats (reset on each metrics report cycle)
}

type grpcReporter struct {
	eventConnection              *grpcConnection // used for events only
	metricConnection             *grpcConnection // used for everything else (postMetrics, postStatus, getSettings)
	eventFlushInterval           int64           // max interval between postEvent batches
	collectMetricInterval        int             // metrics flush interval in seconds
	getSettingsInterval          int             // settings retrieval interval in seconds
	settingsTimeoutCheckInterval int             // check interval for timed out settings in seconds
	collectMetricIntervalLock    sync.RWMutex    // lock to ensure sequential access of collectMetricInterval

	eventMessages  chan []byte      // channel for event messages (sent from agent)
	spanMessages   chan SpanMessage // channel for span messages (sent from agent)
	statusMessages chan []byte      // channel for status messages (sent from agent)
	metricMessages chan []byte      // channel for metrics messages (internal to reporter)
	// The reporter doesn't have a explicit field to record its state. This channel is used to notify all the
	// long-running goroutines to stop themselves. All the goroutines will check this channel and close if the
	// channel is closed.
	// Don't send data into this channel, just close it by calling Shutdown().
	done chan struct{}
	// for testing only: if true, skip verifying TLS cert hostname
	insecureSkipVerify bool
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

	// see if a hostname alias is configured
	configuredHostname = config.GetHostAlias()

	// collector address override
	collectorAddress := config.GetCollector()

	// certificate override
	var cert []byte
	if certPath := config.GetTrustedPath(); certPath != "" {
		var err error
		cert, err = ioutil.ReadFile(certPath)
		if err != nil {
			log.Errorf("Error reading cert file %s: %v", certPath, err)
			return &nullReporter{}
		}
	} else {
		cert = []byte(grpcCertDefault)
	}

	var insecureSkipVerify = config.GetSkipVerify()

	// create connection object for events client and metrics client
	eventConn, err1 := grpcCreateClientConnection(cert, collectorAddress, insecureSkipVerify)
	metricConn, err2 := grpcCreateClientConnection(cert, collectorAddress, insecureSkipVerify)
	if err1 != nil || err2 != nil {
		log.Errorf("Failed to initialize gRPC reporter %v: %v; %v", collectorAddress, err1, err2)
		return &nullReporter{}
	}

	// construct the reporter object which handles two connections
	reporter := &grpcReporter{
		eventConnection: &grpcConnection{
			client:      collector.NewTraceCollectorClient(eventConn),
			connection:  eventConn,
			address:     collectorAddress,
			certificate: cert,
			serviceKey:  serviceKey,
			queueStats:  &eventQueueStats{},
		},
		metricConnection: &grpcConnection{
			client:      collector.NewTraceCollectorClient(metricConn),
			connection:  metricConn,
			address:     collectorAddress,
			certificate: cert,
			serviceKey:  serviceKey,
			queueStats:  &eventQueueStats{},
		},

		eventFlushInterval:           config.ReporterOpts().GetEventFlushInterval(),
		collectMetricInterval:        grpcMetricIntervalDefault,
		getSettingsInterval:          grpcGetSettingsIntervalDefault,
		settingsTimeoutCheckInterval: grpcSettingsTimeoutCheckIntervalDefault,

		eventMessages:  make(chan []byte, 10000),
		spanMessages:   make(chan SpanMessage, 1024),
		statusMessages: make(chan []byte, 1024),
		metricMessages: make(chan []byte, 1024),

		done: make(chan struct{}),

		insecureSkipVerify: insecureSkipVerify,
	}

	// start up long-running goroutine eventSender() which listens on the events message channel
	// and reports incoming events to the collector using GRPC
	go reporter.eventSender()

	// start up long-running goroutine statusSender() which listens on the status message channel
	// and reports incoming events to the collector using GRPC
	go reporter.statusSender()

	// start up long-running goroutine periodicTasks() which kicks off periodic tasks like
	// collectMetrics() and getSettings()
	if !periodicTasksDisabled {
		go reporter.periodicTasks()
	}

	// start up long-running goroutine spanMessageAggregator() which listens on the span message
	// channel and processes incoming span messages
	go reporter.spanMessageAggregator()

	log.Infof("AppOptics reporter is started: %v", reporter.done)
	return reporter
}

// creates a new client connection object which is used by GRPC
// cert		certificate used to verify the collector endpoint
// addr		collector endpoint address and port
//
// returns	client connection object
// 			possible error during AppendCertsFromPEM() and Dial()
func grpcCreateClientConnection(cert []byte, addr string, insecureSkipVerify bool) (*grpc.ClientConn, error) {
	certPool := x509.NewCertPool()

	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		return nil, errors.New("unable to append the certificate to pool")
	}

	// trim port from server name used for TLS verification
	serverName := addr
	if s := strings.Split(addr, ":"); len(s) > 0 {
		serverName = s[0]
	}

	tlsConfig := &tls.Config{
		ServerName:         serverName,
		RootCAs:            certPool,
		InsecureSkipVerify: insecureSkipVerify,
	}
	// turn off server certificate verification for Go < 1.8
	if !isHigherOrEqualGoVersion("go1.8") {
		tlsConfig.InsecureSkipVerify = true
	}
	creds := credentials.NewTLS(tlsConfig)

	return grpc.Dial(addr, grpc.WithTransportCredentials(creds))
}

// Shutdown closes the reporter by close the `done` channel. All long-running goroutines
// monitor the channel `done` in the reporter and close themselves when the channel is closed.
func (r *grpcReporter) Shutdown() error {
	select {
	case <-r.done:
		return ErrShutdownClosedReporter
	default:
		log.Warningf("Shutting down the gRPC reporter: %v", r.done)
		close(r.done)
		// There may be messages
		r.closeConns()
		return nil
	}
}

// closeConns closes all the gRPC connections of a reporter
func (r *grpcReporter) closeConns() {
	r.eventConnection.connection.Close()
	r.metricConnection.connection.Close()
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

// redirect to a different collector
// c			client connection to perform the redirect
// address		redirect address
func (r *grpcReporter) redirect(c *grpcConnection, address string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	log.Debugf("Redirecting to %s", address)

	// Someone else has done it.
	if c.address == address {
		log.Debug("Someone else has done the redirection.")
		return
	}
	// create a new connection object for this client
	conn, err := grpcCreateClientConnection(c.certificate, address, r.insecureSkipVerify)
	if err != nil {
		log.Errorf("Failed redirect to: %v %v", address, err)
	}

	// close the old connection
	c.connection.Close()
	// set new connection (need to be protected)
	c.connection = conn
	c.client = collector.NewTraceCollectorClient(c.connection)
	c.address = address
}

// redirectTo redirects the gRPC connection to the new address and send a
// ConnectionInit message to the collector.
func (r *grpcReporter) redirectTo(c *grpcConnection, address string) {
	r.redirect(c, address)
	c.sendConnectionInit()
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
			go r.eventConnection.ping()
		case <-r.metricConnection.pingTicker.C: // ping on metrics connection (keep alive)
			// set up ticker for next round
			r.metricConnection.resetPing()
			go r.metricConnection.ping()
		case <-r.done:
			return
		}
	}
}

// backoff strategy to slowly increase the retry delay up to a max delay
// oldDelay	the old delay in milliseconds
//
// returns	the new delay in milliseconds
func (r *grpcReporter) setRetryDelay(oldDelay int, retryNum *int) (int, error) {
	if *retryNum > grpcMaxRetries {
		return 0, ErrMaxRetriesExceeded
	}

	newDelay := int(float64(oldDelay) * grpcRetryDelayMultiplier)
	if newDelay > grpcRetryDelayMax*1000 {
		newDelay = grpcRetryDelayMax * 1000
	}
	return newDelay, nil
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
		atomic.AddInt64(&r.eventConnection.queueStats.totalEvents, int64(1)) // use goroutine so this won't block on the critical path
		return nil
	default:
		atomic.AddInt64(&r.eventConnection.queueStats.numOverflowed, int64(1)) // use goroutine so this won't block on the critical path
		return errors.New("event message queue is full")
	}
}

type grpcResult struct {
	ret collector.ResultCode
	err error
}

// eventSender is a long-running goroutine that listens on the events message
// channel, collects all messages on that channel and attempts to send them to
// the collector using the gRPC method PostEvents()
func (r *grpcReporter) eventSender() {
	defer log.Warning("eventSender goroutine exiting.")

	// send connection init message before doing anything else
	r.eventConnection.sendConnectionInit()

	batches := make(chan [][]byte)
	token := r.eventBatchSender(batches)

	flushTk := time.NewTicker(time.Second * time.Duration(r.eventFlushInterval))
	// late binding as flushTk may point to another ticker later
	defer func() { flushTk.Stop() }()

	// This event bucket is drainable either after it reaches HWM, or the flush
	// interval has passed.
	evtBucket := NewBytesBucket(r.eventMessages,
		WithHWM(grpcEventFlushBatchSizeDefault),
		WithTicker(flushTk))

	// The ticker to poll the new EventFlushInterval setting, if any.
	pollTk := time.NewTicker(time.Second)
	defer pollTk.Stop()

	opts := config.ReporterOpts()
	prev := opts.GetUpdateTime()

	for {
		select {
		// Check if the agent is required to quit.
		case <-r.done:
			return
			// dynamically update the event flush interval
		case <-pollTk.C:
			curr := opts.GetUpdateTime()
			if prev != curr {
				prev = curr
				interval := opts.GetEventFlushInterval()
				r.eventFlushInterval = interval

				flushTk.Stop()
				t := time.NewTicker(time.Second * time.Duration(interval))
				flushTk = evtBucket.SetTicker(t)
			}
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
				batches <- evtBucket.Drain()
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
		r.eventRetrySender(batches, token, r.eventConnection)
	}()
	return token
}

func (r *grpcReporter) eventRetrySender(
	batches <-chan [][]byte,
	token chan<- struct{},
	connection *grpcConnection,
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

		connection.queueStats.setQueueLargest(len(messages))

		request := &collector.MessageRequest{
			ApiKey:   connection.serviceKey,
			Messages: messages,
			Encoding: collector.EncodingType_BSON,
		}

		// initial retry delay in milliseconds
		delay := grpcRetryDelayInitial
		// counter for redirects so we know when the limit has been reached
		redirects := 0

		// we'll stay in this loop until the call to PostEvents() succeeds
		resultOK := false
		// Number of gRPC errors encountered
		failsNum := 0
		// Number of retries, including gRPC errors and collector errors
		retriesNum := 0

		for !resultOK {
			// Fail-fast in case the reporter has been closed, avoid retrying in this case.
			select {
			case <-r.done:
				return
			default:
			}
			// protect the call to the client object or we could run into problems if
			// another goroutine is messing with it at the same time, e.g. doing a reconnect()
			connection.lock.RLock()
			response, err := connection.client.PostEvents(context.TODO(), request)
			connection.lock.RUnlock()

			// we sent something, or at least tried to, so we're not idle - reset the keepalive timer
			connection.resetPing()

			if err != nil {
				// gRPC handles the reconnection automatically.
				failsNum++
				if failsNum == grpcRetryLogThreshold {
					log.Warningf("Error calling PostEvents(): %v", err)
				} else {
					log.Debugf("(%v) Error calling PostEvents(): %v", failsNum, err)
				}
			} else {
				if failsNum >= grpcRetryLogThreshold {
					log.Warning("Error recovered in PostEvents()")
				}
				failsNum = 0

				// server responded, check the result code and perform actions accordingly
				switch result := response.GetResult(); result {
				case collector.ResultCode_OK:
					log.Debugf("Sent %d events", len(messages))
					resultOK = true
					atomic.AddInt64(&connection.queueStats.numSent, int64(len(messages)))
					select {
					case token <- struct{}{}:
					case <-r.done:
						return
					}
				case collector.ResultCode_TRY_LATER:
					log.Info("Server responded: Try later")
					atomic.AddInt64(&connection.queueStats.numFailed, int64(len(messages)))
				case collector.ResultCode_LIMIT_EXCEEDED:
					log.Info("Server responded: Limit exceeded")
					atomic.AddInt64(&connection.queueStats.numFailed, int64(len(messages)))
				case collector.ResultCode_INVALID_API_KEY:
					log.Error("Server responded: Invalid API key. Reporter is closing.")
					r.Shutdown()
				case collector.ResultCode_REDIRECT:
					log.Warningf("Reporter is redirecting to %s", response.GetArg())
					if redirects > grpcRedirectMax {
						log.Errorf("Max redirects of %v exceeded", grpcRedirectMax)
					} else {
						r.redirectTo(connection, response.GetArg())
						// a proper redirect shouldn't cause delays
						delay = grpcRetryDelayInitial
						redirects++
					}
				default:
					log.Infof("Unknown Server response in eventRetrySender: %v", result)
				}
			}

			if !resultOK {
				// wait a little before retrying
				time.Sleep(time.Duration(delay) * time.Millisecond)
				if delay, err = r.setRetryDelay(delay, &retriesNum); err != nil {
					break
				}
			}
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

	request := &collector.MessageRequest{
		ApiKey:   r.metricConnection.serviceKey,
		Messages: messages,
		Encoding: collector.EncodingType_BSON,
	}

	// initial retry delay in milliseconds
	delay := grpcRetryDelayInitial
	// counter for redirects so we know when the limit has been reached
	redirects := 0

	// we'll stay in this loop until the call to PostMetrics() succeeds
	resultOK := false
	// Number of gRPC errors encountered
	failsNum := 0
	// Number of retries, including gRPC errors and collector errors
	retriesNum := 0

	// TODO: boilerplate code refactor as events/metrics/settings share similar processes.
	for !resultOK {
		// Fail-fast in case the reporter has been closed, otherwise it may retry until being aborted.
		select {
		case <-r.done:
			return
		default:
		}
		// protect the call to the client object or we could run into problems if
		// another goroutine is messing with it at the same time, e.g. doing a reconnect()
		r.metricConnection.lock.RLock()
		response, err := r.metricConnection.client.PostMetrics(context.TODO(), request)
		r.metricConnection.lock.RUnlock()

		// we sent something, or at least tried to, so we're not idle - reset the keepalive timer
		r.metricConnection.resetPing()

		if err != nil {
			// gRPC handles the reconnection automatically.
			failsNum++
			if failsNum == grpcRetryLogThreshold {
				log.Warningf("Error calling PostMetrics(): %v", err)
			} else {
				log.Debugf("(%v) Error calling PostMetrics(): %v", failsNum, err)
			}
		} else {
			if failsNum >= grpcRetryLogThreshold {
				log.Warning("Error recovered in PostMetrics()")
			}
			failsNum = 0

			// server responded, check the result code and perform actions accordingly
			switch result := response.GetResult(); result {
			case collector.ResultCode_OK:
				log.Debug("Sent metrics")
				resultOK = true
			case collector.ResultCode_TRY_LATER:
				log.Info("Server responded: Try later")
			case collector.ResultCode_LIMIT_EXCEEDED:
				log.Info("Server responded: Limit exceeded")
			case collector.ResultCode_INVALID_API_KEY:
				log.Error("Server responded: Invalid API key. Reporter is closing.")
				r.Shutdown()
			case collector.ResultCode_REDIRECT:
				log.Warningf("Reporter is redirecting to %s", response.GetArg())
				if redirects > grpcRedirectMax {
					log.Errorf("Max redirects of %v exceeded", grpcRedirectMax)
				} else {
					r.redirectTo(r.metricConnection, response.GetArg())
					// a proper redirect shouldn't cause delays
					delay = grpcRetryDelayInitial
					redirects++
				}
			default:
				log.Infof("Unknown Server response in sendMetrics: %v", result)
			}
		}

		if !resultOK {
			// wait a little before retrying
			time.Sleep(time.Duration(delay) * time.Millisecond)
			if delay, err = r.setRetryDelay(delay, &retriesNum); err != nil {
				break
			}
		}
	}
}

// ================================ Settings Handling ====================================

// retrieves the settings from the collector
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) getSettings(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	request := &collector.SettingsRequest{
		ApiKey:        r.metricConnection.serviceKey,
		ClientVersion: grpcReporterVersion,
		Identity: &collector.HostID{
			Hostname:    cachedHostname,
			IpAddresses: getIPAddresses(),
		},
	}

	// initial retry delay in milliseconds
	delay := grpcRetryDelayInitial
	// counter for redirects so we know when the limit has been reached
	redirects := 0

	// we'll stay in this loop until the call to GetSettings() succeeds
	resultOK := false
	// Number of gRPC errors encountered
	failsNum := 0
	// Number of retries, including gRPC errors and collector errors
	retriesNum := 0

	for !resultOK {
		// Fail-fast in case the reporter has been closed, avoid retrying in this case.
		select {
		case <-r.done:
			return
		default:
		}
		// protect the call to the client object or we could run into problems if
		// another goroutine is messing with it at the same time, e.g. doing a reconnect()
		r.metricConnection.lock.RLock()
		response, err := r.metricConnection.client.GetSettings(context.TODO(), request)
		r.metricConnection.lock.RUnlock()

		// we sent something, or at least tried to, so we're not idle - reset the keepalive timer
		r.metricConnection.resetPing()

		if err != nil {
			// gRPC handles the reconnection automatically.
			failsNum++
			if failsNum == grpcRetryLogThreshold {
				log.Warningf("Error calling GetSettings(): %v", err)
			} else {
				log.Debugf("(%v) Error calling GetSettings(): %v", failsNum, err)
			}
		} else {
			if failsNum >= grpcRetryLogThreshold {
				log.Warning("Error recovered in GetSettings()")
			}
			failsNum = 0

			// server responded, check the result code and perform actions accordingly
			switch result := response.GetResult(); result {
			case collector.ResultCode_OK:
				log.Debugf("Got new settings from server %v", r.metricConnection.address)
				r.updateSettings(response)
				resultOK = true
			case collector.ResultCode_TRY_LATER:
				log.Info("Server responded: Try later")
			case collector.ResultCode_LIMIT_EXCEEDED:
				log.Info("Server responded: Limit exceeded")
			case collector.ResultCode_INVALID_API_KEY:
				log.Error("Server responded: Invalid API key. Reporter is closing.")
				r.Shutdown()
			case collector.ResultCode_REDIRECT:
				log.Warningf("Reporter is redirecting to %s", response.GetArg())
				if redirects > grpcRedirectMax {
					log.Errorf("Max redirects of %v exceeded", grpcRedirectMax)
				} else {
					r.redirectTo(r.metricConnection, response.GetArg())
					// a proper redirect shouldn't cause delays
					delay = grpcRetryDelayInitial
					redirects++
				}
			default:
				log.Infof("Unknown Server response in getSettings: %v", result)
			}
		}

		if !resultOK {
			// wait a little before retrying
			time.Sleep(time.Duration(delay) * time.Millisecond)
			if delay, err = r.setRetryDelay(delay, &retriesNum); err != nil {
				break
			}
		}
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
}

// delete settings that have timed out according to their TTL
// ready	a 'ready' channel to indicate if this routine has terminated
func (r *grpcReporter) checkSettingsTimeout(ready chan bool) {
	// notify caller that this routine has terminated (defered to end of routine)
	defer func() { ready <- true }()

	// TODO check TTL
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
	// send connection init message before doing anything else
	r.metricConnection.sendConnectionInit()

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

		request := &collector.MessageRequest{
			ApiKey:   r.metricConnection.serviceKey,
			Messages: messages,
			Encoding: collector.EncodingType_BSON,
		}

		// initial retry delay in milliseconds
		delay := grpcRetryDelayInitial
		// counter for redirects so we know when the limit has been reached
		redirects := 0

		// we'll stay in this loop until the call to PostStatus() succeeds
		resultOK := false
		// Number of gRPC errors encountered
		failsNum := 0
		// Number of retries, including gRPC errors and collector errors
		retriesNum := 0

		for !resultOK {
			// Fail-fast in case the reporter has been closed, avoid retrying in this case.
			select {
			case <-r.done:
				return
			default:
			}
			// protect the call to the client object or we could run into problems if
			// another goroutine is messing with it at the same time, e.g. doing a reconnect()
			r.metricConnection.lock.RLock()
			response, err := r.metricConnection.client.PostStatus(context.TODO(), request)
			r.metricConnection.lock.RUnlock()

			// we sent something, or at least tried to, so we're not idle - reset the keepalive timer
			r.metricConnection.resetPing()

			if err != nil {
				// gRPC handles the reconnection automatically.
				failsNum++
				if failsNum == grpcRetryLogThreshold {
					log.Warningf("Error calling PostStatus(): %v", err)
				} else {
					log.Debugf("(%v) Error calling PostStatus(): %v", failsNum, err)
				}
			} else {
				if failsNum >= grpcRetryLogThreshold {
					log.Warning("Error recovered in PostStatus()")
				}
				failsNum = 0

				// server responded, check the result code and perform actions accordingly
				switch result := response.GetResult(); result {
				case collector.ResultCode_OK:
					log.Debug("Sent status")
					resultOK = true
				case collector.ResultCode_TRY_LATER:
					log.Info("Server responded: Try later")
				case collector.ResultCode_LIMIT_EXCEEDED:
					log.Info("Server responded: Limit exceeded")
				case collector.ResultCode_INVALID_API_KEY:
					log.Error("Server responded: Invalid API key. Reporter is closing.")
					r.Shutdown()
				case collector.ResultCode_REDIRECT:
					log.Warningf("Reporter is redirecting to %s", response.GetArg())
					if redirects > grpcRedirectMax {
						log.Errorf("Max redirects of %v exceeded", grpcRedirectMax)
					} else {
						r.redirectTo(r.metricConnection, response.GetArg())
						// a proper redirect shouldn't cause delays
						delay = grpcRetryDelayInitial
						redirects++
					}
				default:
					log.Infof("Unknown Server response in statusSender: %v", result)
				}
			}

			if !resultOK {
				// wait a little before retrying
				time.Sleep(time.Duration(delay) * time.Millisecond)
				if delay, err = r.setRetryDelay(delay, &retriesNum); err != nil {
					break
				}
			}
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
	c.pingTicker.Reset(time.Duration(grpcPingIntervalDefault) * time.Second)
	c.pingTickerLock.Unlock()
}

// send a keep alive (ping) request on a given GRPC connection
func (c *grpcConnection) ping() {
	request := &collector.PingRequest{
		ApiKey: c.serviceKey,
	}

	c.lock.RLock()
	c.client.Ping(context.TODO(), request)
	c.lock.RUnlock()
}

// ========================= Connection Init Handling =============================

// send a connection init message
func (c *grpcConnection) sendConnectionInit() {
	bbuf := NewBsonBuffer()
	bsonAppendBool(bbuf, "ConnectionInit", true)
	appendHostId(bbuf)
	bsonBufferFinish(bbuf)

	var messages [][]byte
	messages = append(messages, bbuf.buf)

	request := &collector.MessageRequest{
		ApiKey:   c.serviceKey,
		Messages: messages,
		Encoding: collector.EncodingType_BSON,
	}

	c.lock.RLock()
	c.client.PostStatus(context.TODO(), request)
	c.lock.RUnlock()
}
