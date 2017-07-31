// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import "time"

const (
	MetricsRecordMaxSize = 100
)
// MetricsAggregator processes the metrics records and calculate the metrics and
// histogram message from them.
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
	MetricsRaw
}

type MetricsRaw struct {
	histogram Histograms
	Measurements
}

type histogram struct {
	// TODO: use the hdr library
}

type Histograms map[string]Histogram

type Histogram struct {
	hist histogram
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

// FlushBSON is called by the reporter to generate the histogram/metrics
// message in BSON format. It send a request to the histReq channel and
// blocked in the hist channel. FlushBSON is called synchronous so it
// expects to get the result in a short time.
func (am *metricsAggregator) FlushBSON() [][]byte {
	am.rawReq <- struct{}{}
	// Don't let me get blocked here too long
	raw := <- am.raw
	//TODO: encode hist
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

func (am *metricsAggregator) updateMetricsRaw(record MetricsRecord) {
	am.recordHistogram("", record.Duration)
	// TODO
}

func (am *metricsAggregator) recordHistogram(transaction string, duration time.Duration) {
	// TODO
}

func (am *metricsAggregator) processMeasurements(transaction string, record MetricsRecord) {
	// TODO
}

// pushMetricsRaw is called when FlushBSON requires a new histogram message
// for encoding. It push the newest values of the histogram to the hist channel
// which will be consumed by FlushBSON.
func (am *metricsAggregator) pushMetricsRaw() {
	// TODO
}

//TODO: return error types
func (am *metricsAggregator) PushMetricsRecord(record MetricsRecord) bool {
	select {
	case am.records <- record:
		return true
	default:
		return false
	}
}

func newMetricsAggregator() MetricsAggregator {
	return &metricsAggregator{
		records: make(chan MetricsRecord, MetricsRecordMaxSize),
		rawReq: make(chan struct{}),
		raw: make(chan MetricsRaw),
		exit: make(chan struct{}),
	}
}
