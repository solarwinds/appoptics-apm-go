// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"strconv"
	"time"
)

// Some default parameters which may be subject to change.
const (
	MetricsRecordMaxSize      = 100
	MaxTransactionNames       = 200
	DefaultHistogramPrecision = 2
	MaxTagNameLength          = 64
	MaxTagValueLength         = 255
)

// Tags definition
const (
	TAGS_TRANSACTION_NAME = "TransactionName"
	TAGS_HTTP_METHOD      = "HttpMethod"
	TAG_HTTP_STATUS       = "HttpStatus"
	TAGS_ERRORS           = "Errors"
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
	// It passes into the function a pointer but will make a dereference to that
	// pointer when passing into the channel. So it is a separate copy on the other
	// side of the channel.
	PushMetricsRecord(record *MetricsRecord) bool
}

// metricsAggregator is a struct obeying the MetricsAggregator interface.
type metricsAggregator struct {
	// Receive the MetricsRecord sent by traces and update the MetricsRaw.
	// It seems to cost less if we use pointer instead of object here but it's difficult
	// to track/know what will happen to the object itself if the aggregator shares it
	// with the outside caller through a channel, so let's be conservative.
	records chan MetricsRecord
	// Data got from this channel means there is a request from periodic goroutine
	// for a new MetricsRaw struct
	rawReq chan struct{}
	// Send the MetricsRaw through this channel to periodic goroutine
	// It's a chan of struct pointer here as this struct may be big.
	// Make sure to make a deep copy (so not shared with the source end of the chan)
	// before push it into the channel.
	raw chan *MetricsRaw
	// Used to notify the ProcessMetrics goroutine to exit
	exit chan struct{}
	// Stores the seen transaction names, the limit is defined by MaxTransactionNames
	transNames map[string]bool
	// The raw struct of histograms and measurements, it's consumed by FlushBSON to
	// create the metrics message
	// The main goroutine is responsible for updating the metrics in this struct, while
	// the other goroutine requests a **deep copy** of this struct from the main goroutine
	// and encodes the metrics message. The main goroutine needs to reset/clear this
	// struct immediately after send a copy to the metrics-message-encoding goroutine.
	metrics MetricsRaw

	// System metadata cache for metrics messages, which is usually expensive to calculate.
	cachedSysMeta map[string]string
}

// MetricsRaw defines the histograms and measurements maintained by the main goroutine
// which finally are pushed to the BSON encoding/sending goroutine
type MetricsRaw struct {
	histograms   map[string]*Histogram
	measurements map[string]*Measurement
}

// baseHistogram is a the base HDR histogram, an external library.
type baseHistogram struct {
	// TODO: use the hdr library
}

// Histogram contains the data of base histogram and a map of tags
type Histogram struct {
	tags map[string]string
	data baseHistogram
}

// Measurement keeps the tags map, count and sum of the matched request
type Measurement struct {
	tags  map[string]string
	count uint32
	sum   uint64
}

// MetricsRecord is used to collect and transfer the metrics record (http span) by the
// trace agent.
type MetricsRecord struct {
	Transaction string
	Duration    time.Duration
	Status      int
	Method      string
	HasError    bool
}

// FlushBSON is called by the reporter to generate the histograms/metrics
// message in BSON format. It send a request to the histReq channel and
// blocked in the hist channel. FlushBSON is called synchronous so it
// expects to get the result in a short time.
func (am *metricsAggregator) FlushBSON() [][]byte {
	am.rawReq <- struct{}{}
	// Don't let me get blocked here too long
	raw := <-am.raw
	return am.createMetricsMsg(raw)
}

// createMetricsMsg read the histogram and measurement data from MetricsRaw and build
// the BSON message.
func (am *metricsAggregator) createMetricsMsg(raw *MetricsRaw) [][]byte {
	var bbuf bsonBuffer
	bsonBufferInit(&bbuf)

	am.metricsAppendSysMetadata(&bbuf)
	metricsAppendMeasurements(&bbuf, raw)
	metricsAppendHistograms(&bbuf, raw)

	// We don't reset metricAggregator's internal counters (maps or lists) here as it has
	// been done in ProcessMetrics goroutine in a synchronous way for counters consistency.
	bsonBufferFinish(&bbuf)

	var bufs = make([][]byte, 1)
	bufs[0] = bbuf.buf
	return bufs
}

// ProcessMetrics consumes the records sent by traces and update the histograms.
// It also generate and push the metrics event to the hist channel which is consumed by
// FlushBSON to generate the final message in BSON format.
func (am *metricsAggregator) ProcessMetrics() {
	for {
		select {
		case record := <-am.records:
			am.updateMetricsRaw(&record)
		case <-am.rawReq:
			am.pushMetricsRaw()
		case <-am.exit:
			OboeLog(INFO, "ProcessMetrics(): Closing ProcessMetrics goroutine.", nil)
			close(am.raw)
			break
		}
	}
}

// isWithinLimit stores the transaction name into a internal set and returns true, before
// that it checks if the number of transaction names stored inside metricsAggregator is
// still within the limit. If not it returns false and does not store the transaction name.
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

// metricsAggregator updates the Metrics (histograms and measurements) raw data structs
// based on the MetricsRecord.
func (am *metricsAggregator) updateMetricsRaw(record *MetricsRecord) {
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
	var tags = map[string]string{}

	if transaction {
		tags[TAGS_TRANSACTION_NAME] = transaction
	}

	if _, ok := am.metrics.histograms[transaction]; !ok {
		am.metrics.histograms[transaction] = newHistogram(&tags, DefaultHistogramPrecision)
	}
	am.metrics.histograms[transaction].recordValue(uint64(duration.Seconds() * 1e6))
}

// processMeasurements updates the measurements struct based on the new MetricsRecord
func (am *metricsAggregator) processMeasurements(transaction string, record *MetricsRecord) {
	// primary ID: TransactionName
	var primaryTags = map[string]string{}
	primaryTags[TAGS_TRANSACTION_NAME] = transaction
	am.recordMeasurement(&primaryTags, record.Duration)

	// secondary keys: HttpMethod
	var withMethodTags = map[string]string{}
	for k, v := range primaryTags {
		withMethodTags[k] = v
	}
	withMethodTags[TAGS_HTTP_METHOD] = record.Method
	am.recordMeasurement(&withMethodTags, record.Duration)

	// secondary keys: HttpStatus
	var withStatusTags = map[string]string{}
	for k, v := range primaryTags {
		withStatusTags[k] = v
	}
	withStatusTags[TAG_HTTP_STATUS] = strconv.Itoa(record.Status)
	am.recordMeasurement(&withStatusTags, record.Duration)

	// secondary keys: Errors
	if record.HasError {
		var withErrorTags = map[string]string{}
		for k, v := range primaryTags {
			withErrorTags[k] = v
		}
		withErrorTags[TAGS_ERRORS] = "true"
		am.recordMeasurement(&withErrorTags, record.Duration)
	}

}

// recordMeasurement updates a particular measurement based on the tags and duration
func (am *metricsAggregator) recordMeasurement(tags *map[string]string, duration time.Duration) {
	var id string
	for k, v := range *tags {
		id += k + ":" + v + "&"
	}
	if _, ok := am.metrics.measurements[id]; !ok {
		am.metrics.measurements[id] = newMeasurement(tags)
	}
	am.metrics.measurements[id].count++
	am.metrics.measurements[id].sum += uint64(duration.Seconds() * 1e6)
}

// pushMetricsRaw is called when FlushBSON requires a new histograms message
// for encoding. It pushes the newest values of the histograms to the raw channel
// which will be consumed by FlushBSON.
func (am *metricsAggregator) pushMetricsRaw() {
	// Make a deep copy of metrics and reset it immediately, otherwise it will be
	// updated while encoding the message.
	// The following two methods (Copy and resetCounters) should not take too much time,
	// otherwise the records buffered channel may be full in extreme workload.
	var m *MetricsRaw = am.metrics.Copy()
	// Reset the counters for each interval
	am.resetCounters()
	am.raw <- m
}

// Copy makes a copy of this struct and its internal data.
// Don't make the MetricsRaw object too big.
func (m *MetricsRaw) Copy() *MetricsRaw {
	mr := &MetricsRaw{
		histograms:   make(map[string]*Histogram),
		measurements: make(map[string]*Measurement),
	}

	for k, v := range m.histograms {
		mr.histograms[k] = v.Copy()
	}

	for k, v := range m.measurements {
		mr.measurements[k] = v.Copy()
	}

	return mr
}

func (h *Histogram) Copy() *Histogram {
	hCopy := &Histogram{
		tags: make(map[string]string),
		data: h.data.Copy(),
	}

	for k, v := range h.tags {
		hCopy.tags[k] = v
	}

	return hCopy
}

func (m *Measurement) Copy() *Measurement {
	mCopy := &Measurement{
		tags: make(map[string]string),
	}

	for k, v := range m.tags {
		mCopy.tags[k] = v
	}
	mCopy.count = m.count
	mCopy.sum = m.sum

	return mCopy
}

// PushMetricsRecord is called by the Trace to record the metadata of a call, e.g., call duration,
// transaction name, status code.
func (am *metricsAggregator) PushMetricsRecord(record *MetricsRecord) bool {
	select {
	// It makes a copy when *record is passed into the channel, there is no reference types
	// inside MetricsRecord so we don't need a deep copy.
	case am.records <- *record:
		return true
	default:
		return false
	}
}

// recordValue records the duration to the histogram
func (hist *Histogram) recordValue(duration uint64) {
	// TODO: use the API from hdr library
}

// encode is used to encode the histogram into a string
func (hist *Histogram) encode() string {
	return hist.data.encode()
}

func (bh *baseHistogram) Copy() baseHistogram {
	//TODO
}

func newBaseHistogram(precision int) baseHistogram {
	//TODO
}

// encode is a wrapper of hdr's function with (probably) the same name
func (h *baseHistogram) encode() (str string) {
	// TODO
	return str //TODO remove it
}

// newHistogram creates a Histogram object with tags and precision
func newHistogram(inTags *map[string]string, precision int) *Histogram {
	var histogram = Histogram{
		tags: make(map[string]string),
		data: newBaseHistogram(precision),
	}
	for k, v := range *inTags {
		histogram.tags[k] = v
	}
	return &histogram
}

// newMeasurement creates a Measurement object with tags
func newMeasurement(inTags *map[string]string) *Measurement {
	var measurement = Measurement{
		tags: make(map[string]string),
	}
	for k, v := range *inTags {
		measurement.tags[k] = v
	}
	return &measurement
}

// newMetricsAggregator is the newMetricsAggregator initializer. Note: You still need to
// initialize the Hisogram.data each time you add a new key/value to it, as by default
// it's a nil map pointer.
func newMetricsAggregator() MetricsAggregator {
	return &metricsAggregator{
		records:    make(chan MetricsRecord, MetricsRecordMaxSize),
		rawReq:     make(chan struct{}),
		raw:        make(chan *MetricsRaw),
		exit:       make(chan struct{}),
		transNames: make(map[string]bool),
		metrics: MetricsRaw{
			histograms:   make(map[string]*Histogram),
			measurements: make(map[string]*Measurement),
		},
		cachedSysMeta: make(map[string]string),
	}
}
