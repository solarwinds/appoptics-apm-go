package traceview

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/librato/go-traceview/v1/tv/internal/traceview/collector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	grpcReporterVersion = "golang-v2"
	grpcAddressDefault  = "ec2-54-175-46-34.compute-1.amazonaws.com:5555"
	grpcCertDefault     = `-----BEGIN CERTIFICATE-----
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

	grpcMetricIntervalDefault      = 5   // seconds
	grpcGetSettingsIntervalDefault = 30  // seconds
	grpcRetryDelayInitial          = 500 // miliseconds
	grpcRetryDelayMultiplier       = 1.5 //
	grpcRetryDelayMax              = 60  //seconds
	grpcRedirectMax                = 20
)

type reconnectAuthority int

const (
	UNSET reconnectAuthority = iota
	POSTEVENTS
	POSTMETRICS
	GETSETTINGS
)

type connection struct {
	client             collector.TraceCollectorClient
	connection         *grpc.ClientConn
	address            string
	certificate        []byte
	lock               sync.Mutex
	reconnectAuthority reconnectAuthority
}

type grpcReporter struct {
	eventConnection       connection
	metricConnection      connection
	serviceKey            string
	collectMetricInterval int
	getSettingsInterval   int
}

var grpcEventMessages = make(chan []byte, 1024)
var grpcMetricMessages = make(chan []byte, 1024)
var grpcSpanMessages = make(chan HttpSpanMessage, 1024)

func grpcNewReporter() Reporter {
	serviceKey := os.Getenv("APPOPTICS_SERVICE_KEY")
	if serviceKey == "" {
		OboeLog(WARNING, "No service key found, check environment variable APPOPTICS_SERVICE_KEY.")
		return &nullReporter{}
	}

	collectorAddress := os.Getenv("APPOPTICS_COLLECTOR")
	if collectorAddress == "" {
		collectorAddress = grpcAddressDefault
	}

	var cert []byte
	if certPath := os.Getenv("APPOPTICS_TRUSTEDPATH"); certPath != "" {
		var err error
		cert, err = ioutil.ReadFile(certPath)
		if err != nil {
			OboeLog(ERROR, fmt.Sprintf("Error reading cert file %s: %v", certPath, err))
			return &nullReporter{}
		}
	} else {
		cert = []byte(grpcCertDefault)
	}

	conn1, err1 := grpcCreateClientConnection(cert, collectorAddress)
	conn2, err2 := grpcCreateClientConnection(cert, collectorAddress)
	if err1 != nil || err2 != nil {
		var err error
		switch {
		case err1 != nil:
			err = err1
		case err2 != nil:
			err = err2
		}
		OboeLog(ERROR, fmt.Sprintf("Failed to initialize gRPC reporter: %v %v", collectorAddress, err))
		return &nullReporter{}
	}

	reporter := &grpcReporter{
		eventConnection: connection{
			client:      collector.NewTraceCollectorClient(conn1),
			connection:  conn1,
			address:     collectorAddress,
			certificate: cert,
		},
		metricConnection: connection{
			client:      collector.NewTraceCollectorClient(conn2),
			connection:  conn2,
			address:     collectorAddress,
			certificate: cert,
		},
		serviceKey: serviceKey,

		collectMetricInterval: grpcMetricIntervalDefault,
		getSettingsInterval:   grpcGetSettingsIntervalDefault,
	}

	go reporter.eventSender()
	go reporter.periodicTasks()
	go reporter.spanMessageAggregator()
	return reporter
}

func grpcCreateClientConnection(cert []byte, addr string) (*grpc.ClientConn, error) {
	certPool := x509.NewCertPool()

	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		return nil, errors.New("Unable to append the certificate to pool.")
	}

	creds := credentials.NewTLS(&tls.Config{
		ServerName:         addr,
		RootCAs:            certPool,
		InsecureSkipVerify: true, // TODO: a workaround, don't turn it on for production.
	})

	return grpc.Dial(addr, grpc.WithTransportCredentials(creds))
}

func (r *grpcReporter) reconnect(c *connection, authority reconnectAuthority) {
	if c.reconnectAuthority == UNSET {
		c.lock.Lock()
		if c.reconnectAuthority == UNSET {
			c.reconnectAuthority = authority
		}
		c.lock.Unlock()
	}

	if c.reconnectAuthority == authority {
		c.lock.Lock()
		OboeLog(INFO, "Lost connection -- attempting reconnect...")
		c.client = collector.NewTraceCollectorClient(c.connection)
		c.lock.Unlock()
	} else {
		for c.reconnectAuthority != UNSET {
			time.Sleep(time.Second)
		}
	}
}

func (r *grpcReporter) redirect(c *connection, authority reconnectAuthority, address string) {
	conn, err := grpcCreateClientConnection(c.certificate, address)
	if err != nil {
		OboeLog(ERROR, fmt.Sprintf("Failed redirect to: %v %v", address, err))
	}

	c.lock.Lock()
	c.connection = conn
	c.lock.Unlock()

	r.reconnect(c, authority)
}

func (r *grpcReporter) periodicTasks() {
	//	collectMetricsTicker := time.NewTimer(r.getMetricsNextInterval())
	collectMetricsTicker := time.NewTimer(0)
	getSettingsTicker := time.NewTimer(0)

	collectMetricsReady := make(chan bool, 1)
	sendMetricsReady := make(chan bool, 1)
	getSettingsReady := make(chan bool, 1)
	collectMetricsReady <- true
	sendMetricsReady <- true
	getSettingsReady <- true

	for {
		select {

		case <-collectMetricsTicker.C:
			collectMetricsTicker.Reset(r.collectMetricsNextInterval())
			select {
			case <-collectMetricsReady:
				go r.collectMetrics(collectMetricsReady, sendMetricsReady)
			default:
			}

		case <-getSettingsTicker.C:
			getSettingsTicker.Reset(time.Duration(r.getSettingsInterval) * time.Second)
			select {
			case <-getSettingsReady:
				go r.getSettings(getSettingsReady)
			default:
			}
		}
	}
}

func (r *grpcReporter) setRetryDelay(delay *int) {
	*delay = int(float64(*delay) * grpcRetryDelayMultiplier)
	if *delay > grpcRetryDelayMax*1000 {
		*delay = grpcRetryDelayMax * 1000
	}
}

// ================================ Event Handling ====================================

func (r *grpcReporter) ReportEvent(ctx *oboeContext, e *event) error {
	if err := prepareEvent(ctx, e); err != nil {
		return err
	}

	select {
	case grpcEventMessages <- (*e).bbuf.GetBuf():
		go incrementTotalEvents(1) // use goroutine so this won't block on the critical path
		return nil
	default:
		go incrementNumOverflowed(1) // use goroutine so this won't block on the critical path
		return errors.New("Event message queue is full")
	}
}

func (r *grpcReporter) eventSender() {
	for {
		var messages [][]byte

		for len(messages) == 0 {
			done := false
			for !done {
				select {
				case e := <-grpcEventMessages:
					messages = append(messages, e)
				default:
					done = true
				}
			}

			if len(messages) == 0 {
				time.Sleep(time.Duration(500) * time.Millisecond)
			}
		}

		setQueueLargest(len(messages))

		//		for _, aaa := range messages {
		//			printBson(aaa)
		//		}

		request := &collector.MessageRequest{
			ApiKey:   r.serviceKey,
			Messages: messages,
			Encoding: collector.EncodingType_BSON,
		}

		delay := grpcRetryDelayInitial
		redirects := 0

		resultOk := false
		for !resultOk {
			r.eventConnection.lock.Lock()
			response, err := r.eventConnection.client.PostEvents(context.TODO(), request)
			r.eventConnection.lock.Unlock()

			if err != nil {
				r.reconnect(&r.eventConnection, POSTEVENTS)
			} else {
				switch result := response.GetResult(); result {
				case collector.ResultCode_OK:
					OboeLog(DEBUG, "Sent events")
					resultOk = true
					r.eventConnection.reconnectAuthority = UNSET
					incrementNumSent(len(messages))
				case collector.ResultCode_TRY_LATER:
					OboeLog(DEBUG, "Server responded: Try later")
					incrementNumFailed(len(messages))
				case collector.ResultCode_LIMIT_EXCEEDED:
					OboeLog(DEBUG, "Server responded: Limit exceeded")
					incrementNumFailed(len(messages))
				case collector.ResultCode_INVALID_API_KEY:
					OboeLog(DEBUG, "Server responded: Invalid API key")
				case collector.ResultCode_REDIRECT:
					if redirects > grpcRedirectMax {
						OboeLog(ERROR, fmt.Sprintf("Max redirects of %v exceeded", grpcRedirectMax))
					} else {
						r.redirect(&r.eventConnection, POSTEVENTS, response.Arg)
						delay = grpcRetryDelayInitial
						redirects++
					}
				default:
					OboeLog(DEBUG, "Unknown Server response")
				}
			}

			if !resultOk {
				time.Sleep(time.Duration(delay) * time.Millisecond)
				r.setRetryDelay(&delay)
			}
		}
	}
}

// ================================ Metrics Handling ====================================

func (r *grpcReporter) collectMetricsNextInterval() time.Duration {
	interval := r.collectMetricInterval - (time.Now().Second() % r.collectMetricInterval)
	return time.Duration(interval) * time.Second
}

func (r *grpcReporter) collectMetrics(collectReady chan bool, sendReady chan bool) {
	defer func() { collectReady <- true }()

	message := generateMetricsMessage(r.collectMetricInterval)
	//	printBson(message)

	select {
	case grpcMetricMessages <- message:
	default:
	}

	select {
	case <-sendReady:
		go r.sendMetrics(sendReady)
	default:
	}
}

func (r *grpcReporter) sendMetrics(ready chan bool) {
	defer func() { ready <- true }()

	var messages [][]byte

	done := false
	for !done {
		select {
		case m := <-grpcMetricMessages:
			messages = append(messages, m)
		default:
			done = true
		}
	}

	request := &collector.MessageRequest{
		ApiKey:   r.serviceKey,
		Messages: messages,
		Encoding: collector.EncodingType_BSON,
	}

	delay := grpcRetryDelayInitial
	redirects := 0

	resultOk := false
	for !resultOk {
		r.metricConnection.lock.Lock()
		response, err := r.metricConnection.client.PostMetrics(context.TODO(), request)
		r.metricConnection.lock.Unlock()

		if err != nil {
			r.reconnect(&r.metricConnection, POSTMETRICS)
		} else {
			switch result := response.GetResult(); result {
			case collector.ResultCode_OK:
				OboeLog(DEBUG, "Sent metrics")
				resultOk = true
				r.metricConnection.reconnectAuthority = UNSET
			case collector.ResultCode_TRY_LATER:
				OboeLog(DEBUG, "Server responded: Try later")
			case collector.ResultCode_LIMIT_EXCEEDED:
				OboeLog(DEBUG, "Server responded: Limit exceeded")
			case collector.ResultCode_INVALID_API_KEY:
				OboeLog(DEBUG, "Server responded: Invalid API key")
			case collector.ResultCode_REDIRECT:
				if redirects > grpcRedirectMax {
					OboeLog(ERROR, fmt.Sprintf("Max redirects of %v exceeded", grpcRedirectMax))
				} else {
					r.redirect(&r.metricConnection, POSTMETRICS, response.Arg)
					delay = grpcRetryDelayInitial
					redirects++
				}
			default:
				OboeLog(DEBUG, "Unknown Server response")
			}
		}

		if !resultOk {
			time.Sleep(time.Duration(delay) * time.Millisecond)
			r.setRetryDelay(&delay)
		}
	}
}

// ================================ Settings Handling ====================================

func (r *grpcReporter) getSettings(ready chan bool) {
	defer func() { ready <- true }()

	var ipAddrs []string
	var uuid string
	ipAddrs = nil
	uuid = ""

	request := &collector.SettingsRequest{
		ApiKey:        r.serviceKey,
		ClientVersion: grpcReporterVersion,
		Identity: &collector.HostID{
			Hostname:    cachedHostname,
			IpAddresses: ipAddrs,
			Uuid:        uuid,
		},
	}

	delay := grpcRetryDelayInitial
	redirects := 0

	resultOK := false
	for !resultOK {
		r.metricConnection.lock.Lock()
		response, err := r.metricConnection.client.GetSettings(context.TODO(), request)
		r.metricConnection.lock.Unlock()

		if err != nil {
			r.reconnect(&r.metricConnection, GETSETTINGS)
		} else {
			switch result := response.GetResult(); result {
			case collector.ResultCode_OK:
				OboeLog(DEBUG, fmt.Sprintf("Got new settings from server %v", r.metricConnection.address))
				r.updateSettings(response)
				resultOK = true
				r.metricConnection.reconnectAuthority = UNSET
			case collector.ResultCode_TRY_LATER:
				OboeLog(DEBUG, "Server responded: Try later")
			case collector.ResultCode_LIMIT_EXCEEDED:
				OboeLog(DEBUG, "Server responded: Limit exceeded")
			case collector.ResultCode_INVALID_API_KEY:
				OboeLog(DEBUG, "Server responded: Invalid API key")
			case collector.ResultCode_REDIRECT:
				if redirects > grpcRedirectMax {
					OboeLog(ERROR, fmt.Sprintf("Max redirects of %v exceeded", grpcRedirectMax))
				} else {
					r.redirect(&r.metricConnection, GETSETTINGS, response.Arg)
					delay = grpcRetryDelayInitial
					redirects++
				}
			default:
				OboeLog(DEBUG, "Unknown Server response")
			}
		}

		if !resultOK {
			time.Sleep(time.Duration(delay) * time.Millisecond)
			r.setRetryDelay(&delay)
		}
	}

}

func (r *grpcReporter) updateSettings(settings *collector.SettingsResult) {
	for _, s := range settings.Settings {
		//TODO save new settings
		fmt.Println(s)
	}
}

// ========================= Span Message Handling =============================

func (r *grpcReporter) ReportSpan(span *HttpSpanMessage) error {
	select {
	case grpcSpanMessages <- *span:
		return nil
	default:
		return errors.New("Span message queue is full")
	}
}

func (r *grpcReporter) spanMessageAggregator() {
	for {
		select {
		case span := <-grpcSpanMessages:
			recordHistogram(metricsHTTPHistograms, "", span.Duration)

			if span.Transaction == "" && span.Url != "" {
				span.Transaction = getTransactionFromURL(span.Url)
			}
			if span.Transaction != "" {
				transactionWithinLimit := isWithinLimit(
					&metricsHTTPTransactions, span.Transaction, metricsHTTPTransactionsMax)

				if transactionWithinLimit {
					recordHistogram(metricsHTTPHistograms, span.Transaction, span.Duration)
					processHttpMeasurements(span.Transaction, &span)
				} else {
					processHttpMeasurements("other", &span)
					setTransactionNameOverflow(true)
				}
			} else {
				processHttpMeasurements("unknown", &span)
			}
		}
	}
}
