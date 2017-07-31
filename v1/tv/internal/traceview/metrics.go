// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import "time"

const (
	MetricsRecordMaxSize = 100
	MaxTransactionNames = 200
)
// MetricsAggregator processes the metrics records and calculate the metrics and
// histograms message from them.
type MetricsAggregator interface {
	// FlushBSON requests a metrics message from the message channel and encode
	// it into a BSON message.
	FlushBSON() [][]byte
	// ProcessMetrics consumes the metrics records from the records channel
	// and update the histograms and metrics based on the records. It will
	// send a metrics message to the message channel on request and may reset
	// the histograms and metrics. It is started as a separate goroutine.
	ProcessMetrics()
	// PushMetricsRecord push a metrics record to the records channel and return
	// immediately if the channel is full.
	PushMetricsRecord(record MetricsRecord) bool
}

type metricsAggregator struct {
	records chan MetricsRecord
	rawReq chan struct{}
	raw chan MetricsRaw
	exit chan struct{}
	transNames map[string]bool
	metrics MetricsRaw
}

type MetricsRaw struct {
	histograms map[string]Histogram
	measurements Measurements
}

type baseHistogram struct {
	// TODO: use the hdr library
}

type Histogram struct {
	data baseHistogram
	tags map[string]string
}

type Measurements struct {
	// TODO
}

type MetricsRecord struct {
	Transaction string
	Duration time.Duration
	Status int
	Method string
	HasError bool
}

// FlushBSON is called by the reporter to generate the histograms/metrics
// message in BSON format. It send a request to the histReq channel and
// blocked in the hist channel. FlushBSON is called synchronous so it
// expects to get the result in a short time.
func (am *metricsAggregator) FlushBSON() [][]byte {
	am.rawReq <- struct{}{}
	// Don't let me get blocked here too long
	raw := <- am.raw
	return am.createMetricsMsg(raw)
}

// createMetricsMsg read the histogram and measurement data from MetricsRaw and build
// the BSON message.
func (am *metricsAggregator) createMetricsMsg(raw MetricsRaw) [][]byte {
	// TODO
}

// ProcessMetrics consumes the records sent by traces and update the histograms.
// It also generate and push the metrics event to the hist channel which is consumed by
// FlushBSON to generate the final message in BSON format.
func (am *metricsAggregator) ProcessMetrics() {
	for {
		select {
		case record := <-am.records:
			am.updateMetricsRaw(record)
		case <- am.rawReq:
			am.pushMetricsRaw()
		case <- am.exit:
			OboeLog(INFO, "Closing ProcessMetrics goroutine.", nil)
			close(am.raw)
			break
		}
	}
}

// isWithinLimit stores the transaction name into a internal set and returns true, before that
// it checks if the number of transaction names stored inside metricsAggregator is still within
// the limit. If not it returns false and does not store the transaction name.
func (am *metricsAggregator) isWithinLimit(transaction string, max int) bool {
	if _, ok := am.transNames[transaction]; !ok {
		if len(am.transNames) < max {
			am.transNames[transaction] = true
			return true
		} else {
			return false
		}
	}
	return true
}

// metricsAggregator updates the Metrics (histograms and measurements) raw data structs based on
// the MetricsRecord.
func (am *metricsAggregator) updateMetricsRaw(record MetricsRecord) {
	am.recordHistogram("", record.Duration)
	if record.Transaction {
		if am.isWithinLimit(record.Transaction, MaxTransactionNames) {
			am.recordHistogram(record.Transaction, record.Duration)
			am.processMeasurements(record.Transaction, record)
		} else {
			am.processMeasurements("other", record)
		}
	} else {
		am.processMeasurements("unknown", record)
	}
}

// recordHistogram updates the histogram based on the new MetricsRecord (transaction name and
// the duration).
func (am *metricsAggregator) recordHistogram(transaction string, duration time.Duration) {
	// TODO
}

// processMeasurements updates the measurements struct based on the new MetricsRecord
func (am *metricsAggregator) processMeasurements(transaction string, record MetricsRecord) {
	// TODO
}

// pushMetricsRaw is called when FlushBSON requires a new histograms message
// for encoding. It pushes the newest values of the histograms to the raw channel
// which will be consumed by FlushBSON.
func (am *metricsAggregator) pushMetricsRaw() {
	// TODO
	am.raw <- am.metrics
}

// PushMetricsRecord is called by the Trace to record the metadata of a call, e.g., call duration,
// transaction name, status code.
func (am *metricsAggregator) PushMetricsRecord(record MetricsRecord) bool {
	select {
	case am.records <- record:
		return true
	default:
		return false
	}
}

// newMetricsAggregator is the newMetricsAggregator initializer. Note: You still need to
// initialize the Hisogram.data each time you add a new key/value to it, as by default
// it's a nil map pointer.
func newMetricsAggregator() MetricsAggregator {
	return &metricsAggregator{
		records: make(chan MetricsRecord, MetricsRecordMaxSize),
		rawReq: make(chan struct{}),
		raw: make(chan MetricsRaw),
		exit: make(chan struct{}),
		transNames: make(map[string]bool),
		metrics: MetricsRaw{histograms: make(map[string]Histogram)},
	}
}
