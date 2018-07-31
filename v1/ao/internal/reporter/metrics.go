// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/hdrhist"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/host"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
)

// Linux distributions and their identifying files
const (
	metricsTransactionsMaxDefault = 200 // default max amount of transaction names we allow per cycle
	metricsHistPrecisionDefault   = 2   // default histogram precision

	metricsTagNameLenghtMax  = 64  // max number of characters for tag names
	metricsTagValueLenghtMax = 255 // max number of characters for tag values
)

// Special transaction names
const (
	UnknownTransactionName       = "unknown"
	OtherTransactionName         = "other"
	maxPathLenForTransactionName = 3
)

// SpanMessage defines a span message
type SpanMessage interface {
	// called for message processing
	process()
}

// BaseSpanMessage is the base span message with properties found in all types of span messages
type BaseSpanMessage struct {
	Duration time.Duration // duration of the span (nanoseconds)
	HasError bool          // boolean flag whether this transaction contains an error or not
}

// HTTPSpanMessage is used for inbound metrics
type HTTPSpanMessage struct {
	BaseSpanMessage
	Transaction string // transaction name (e.g. controller.action)
	Path        string // the url path which will be processed and used as transaction (if Transaction is empty)
	Status      int    // HTTP status code (e.g. 200, 500, ...)
	Host        string // HTTP-Host
	Method      string // HTTP method (e.g. GET, POST, ...)
}

// Measurement is a single measurement for reporting
type Measurement struct {
	Name      string            // the name of the measurement (e.g. TransactionResponseTime)
	Tags      map[string]string // map of KVs
	Count     int               // count of this measurement
	Sum       float64           // sum for this measurement
	ReportSum bool              // include the sum in the report?
}

// a collection of measurements
type measurements struct {
	measurements map[string]*Measurement
	lock         sync.Mutex // protect access to this collection
}

// a single histogram
type histogram struct {
	hist *hdrhist.Hist     // internal representation of a histogram (see hdrhist package)
	tags map[string]string // map of KVs
}

// a collection of histograms
type histograms struct {
	histograms map[string]*histogram
	precision  int        // histogram precision (a value between 0-5)
	lock       sync.Mutex // protect access to this collection
}

// counters of the event queue stats
// All the fields are supposed to be accessed through atomic operations
type eventQueueStats struct {
	numSent       int64 // number of messages that were successfully sent
	numOverflowed int64 // number of messages that overflowed the queue
	numFailed     int64 // number of messages that failed to send
	totalEvents   int64 // number of messages queued to send
	queueLargest  int64 // maximum number of messages that were in the queue at one time
}

// rate counts reported by trace sampler
type rateCounts struct{ requested, sampled, limited, traced, through int64 }

// TransMap records the received transaction names in a metrics report cycle. It will refuse
// new transaction names if reaching the capacity.
type TransMap struct {
	// The map to store transaction names
	transactionNames map[string]struct{}
	// The maximum capacity of the transaction map. The value is got from server settings which
	// is updated periodically.
	// The default value metricsTransactionsMaxDefault is used when a new TransMap
	// is initialized.
	currCap int
	// The maximum capacity which is set by the server settings. This update usually happens in
	// between two metrics reporting cycles. To avoid affecting the map capacity of the current reporting
	// cycle, the new capacity got from the server is stored in nextCap and will only be flushed to currCap
	// when the Reset() is called.
	nextCap int
	// Whether there is an overflow. Overflow means the user tried to store more transaction names
	// than the capacity defined by settings.
	// This flag is cleared in every metrics cycle.
	overflow bool
	// The mutex to protect this whole struct. If the performance is a concern we should use separate
	// mutexes for each of the fields. But for now it seems not necessary.
	mutex *sync.Mutex
}

// NewTransMap initializes a new TransMap struct
func NewTransMap(cap int) *TransMap {
	return &TransMap{
		transactionNames: make(map[string]struct{}),
		currCap:          cap,
		nextCap:          cap,
		overflow:         false,
		mutex:            &sync.Mutex{},
	}
}

// SetCap sets the capacity of the transaction map
func (t *TransMap) SetCap(cap int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.nextCap = cap
}

// ResetTransMap resets the transaction map to a initialized state. The new capacity got from the
// server will be used in next metrics reporting cycle after reset.
func (t *TransMap) Reset() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.transactionNames = make(map[string]struct{})
	t.currCap = t.nextCap
	t.overflow = false
}

// IsWithinLimit checks if the transaction name is stored in the TransMap. It will store this new
// transaction name and return true if not stored before and the map isn't full, or return false
// otherwise.
func (t *TransMap) IsWithinLimit(name string) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, ok := t.transactionNames[name]; !ok {
		// only record if we haven't reached the limits yet
		if len(t.transactionNames) < t.currCap {
			t.transactionNames[name] = struct{}{}
			return true
		}
		t.overflow = true
		return false
	}

	return true
}

// Overflow returns true is the transaction map is overflow (reached its limit)
// or false if otherwise.
func (t *TransMap) Overflow() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.overflow
}

// mTransMap is the list of currently stored unique HTTP transaction names
// (flushed on each metrics report cycle)
var mTransMap = NewTransMap(metricsTransactionsMaxDefault)

// collection of currently stored measurements (flushed on each metrics report cycle)
var metricsHTTPMeasurements = &measurements{
	measurements: make(map[string]*Measurement),
}

// collection of currently stored histograms (flushed on each metrics report cycle)
var metricsHTTPHistograms = &histograms{
	histograms: make(map[string]*histogram),
	precision:  metricsHistPrecisionDefault,
}

// TODO: use config package, and add validator (0-5)
// initialize values according to env variables
func init() {
	pEnv := "APPOPTICS_HISTOGRAM_PRECISION"
	precision := os.Getenv(pEnv)
	if precision != "" {
		log.Infof("Non-default APPOPTICS_HISTOGRAM_PRECISION: %s", precision)
		if p, err := strconv.Atoi(precision); err == nil {
			if p >= 0 && p <= 5 {
				metricsHTTPHistograms.precision = p
			} else {
				log.Errorf("value of %v must be between 0 and 5: %v", pEnv, precision)
			}
		} else {
			log.Errorf("value of %v is not an int: %v", pEnv, precision)
		}
	}
}

// generates a metrics message in BSON format with all the currently available values
// metricsFlushInterval	current metrics flush interval
//
// return				metrics message in BSON format
func generateMetricsMessage(metricsFlushInterval int, queueStats *eventQueueStats) []byte {
	bbuf := NewBsonBuffer()

	appendHostId(bbuf)
	bsonAppendInt64(bbuf, "Timestamp_u", int64(time.Now().UnixNano()/1000))
	bsonAppendInt(bbuf, "MetricsFlushInterval", metricsFlushInterval)

	// measurements
	// ==========================================
	start := bsonAppendStartArray(bbuf, "measurements")
	index := 0

	// request counters
	rc := flushRateCounts()
	addMetricsValue(bbuf, &index, "RequestCount", rc.requested)
	addMetricsValue(bbuf, &index, "TraceCount", rc.traced)
	addMetricsValue(bbuf, &index, "TokenBucketExhaustionCount", rc.limited)
	addMetricsValue(bbuf, &index, "SampleCount", rc.sampled)
	addMetricsValue(bbuf, &index, "ThroughTraceCount", rc.through)

	// Queue states
	q := queueStats.copyAndReset()
	addMetricsValue(bbuf, &index, "NumSent", q.numSent)
	addMetricsValue(bbuf, &index, "NumOverflowed", q.numOverflowed)
	addMetricsValue(bbuf, &index, "NumFailed", q.numFailed)
	addMetricsValue(bbuf, &index, "TotalEvents", q.totalEvents)
	addMetricsValue(bbuf, &index, "QueueLargest", q.queueLargest)

	addHostMetrics(bbuf, &index)

	// runtime stats
	addMetricsValue(bbuf, &index, "JMX.type=threadcount,name=NumGoroutine", runtime.NumGoroutine())
	var mem runtime.MemStats
	host.Mem(&mem)
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Alloc", int64(mem.Alloc))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.TotalAlloc", int64(mem.TotalAlloc))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Sys", int64(mem.Sys))
	addMetricsValue(bbuf, &index, "JMX.Memory:type=count,name=MemStats.Lookups", int64(mem.Lookups))
	addMetricsValue(bbuf, &index, "JMX.Memory:type=count,name=MemStats.Mallocs", int64(mem.Mallocs))
	addMetricsValue(bbuf, &index, "JMX.Memory:type=count,name=MemStats.Frees", int64(mem.Frees))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Heap.Alloc", int64(mem.HeapAlloc))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Heap.Sys", int64(mem.HeapSys))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Heap.Idle", int64(mem.HeapIdle))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Heap.Inuse", int64(mem.HeapInuse))
	addMetricsValue(bbuf, &index, "JMX.Memory:MemStats.Heap.Released", int64(mem.HeapReleased))
	addMetricsValue(bbuf, &index, "JMX.Memory:type=count,name=MemStats.Heap.Objects", int64(mem.HeapObjects))
	var gc debug.GCStats
	host.GC(&gc)
	addMetricsValue(bbuf, &index, "JMX.type=count,name=GCStats.NumGC", gc.NumGC)

	metricsHTTPMeasurements.lock.Lock()
	for _, m := range metricsHTTPMeasurements.measurements {
		addMeasurementToBSON(bbuf, &index, m)
	}
	metricsHTTPMeasurements.measurements = make(map[string]*Measurement) // clear measurements
	metricsHTTPMeasurements.lock.Unlock()

	bsonAppendFinishObject(bbuf, start)
	// ==========================================

	// histograms
	// ==========================================
	start = bsonAppendStartArray(bbuf, "histograms")
	index = 0

	metricsHTTPHistograms.lock.Lock()

	for _, h := range metricsHTTPHistograms.histograms {
		addHistogramToBSON(bbuf, &index, h)
	}
	metricsHTTPHistograms.histograms = make(map[string]*histogram) // clear histograms

	metricsHTTPHistograms.lock.Unlock()
	bsonAppendFinishObject(bbuf, start)
	// ==========================================

	if mTransMap.Overflow() {
		bsonAppendBool(bbuf, "TransactionNameOverflow", true)
	}
	// The transaction map is reset in every metrics cycle.
	mTransMap.Reset()

	bsonBufferFinish(bbuf)
	return bbuf.buf
}

// append host ID to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendHostId(bbuf *bsonBuffer) {
	if host.ConfiguredHostname() != "" {
		bsonAppendString(bbuf, "ConfiguredHostname", host.ConfiguredHostname())
	}
	appendUname(bbuf)
	bsonAppendString(bbuf, "Distro", host.Distro())
	appendIPAddresses(bbuf)
}

// gets and appends IP addresses to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendIPAddresses(bbuf *bsonBuffer) {
	addrs := host.IPAddresses()
	if addrs == nil {
		return
	}

	start := bsonAppendStartArray(bbuf, "IPAddresses")
	for i, address := range addrs {
		bsonAppendString(bbuf, strconv.Itoa(i), address)
	}
	bsonAppendFinishObject(bbuf, start)
}

// gets and appends MAC addresses to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendMACAddresses(bbuf *bsonBuffer, macs []string) {
	start := bsonAppendStartArray(bbuf, "MACAddresses")
	for _, mac := range macs {
		if mac == "" {
			continue
		}
		i := 0
		bsonAppendString(bbuf, strconv.Itoa(i), mac)
		i++
	}
	bsonAppendFinishObject(bbuf, start)
}

// appends a metric to a BSON buffer, the form will be:
// {
//   "name":"myName",
//   "value":0
// }
// bbuf		the BSON buffer to append the metric to
// index	a running integer (0,1,2,...) which is needed for BSON arrays
// name		key name
// value	value (type: int, int64, float32, float64)
func addMetricsValue(bbuf *bsonBuffer, index *int, name string, value interface{}) {
	start := bsonAppendStartObject(bbuf, strconv.Itoa(*index))
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("%v", err)
		}
	}()

	bsonAppendString(bbuf, "name", name)
	switch value.(type) {
	case int:
		bsonAppendInt(bbuf, "value", value.(int))
	case int64:
		bsonAppendInt64(bbuf, "value", value.(int64))
	case float32:
		v32 := value.(float32)
		v64 := float64(v32)
		bsonAppendFloat64(bbuf, "value", v64)
	case float64:
		bsonAppendFloat64(bbuf, "value", value.(float64))
	default:
		bsonAppendString(bbuf, "value", "unknown")
	}

	bsonAppendFinishObject(bbuf, start)
	*index += 1
}

// GetTransactionFromPath performs fingerprinting on a given escaped path to extract the transaction name
// We can get the path so there is no need to parse the full URL.
// e.g. Escaped Path path: /appoptics/appoptics-apm-go/blob/metrics becomes /appoptics/appoptics-apm-go
func GetTransactionFromPath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	p := strings.Split(path, "/")
	lp := len(p)
	if lp > maxPathLenForTransactionName {
		lp = maxPathLenForTransactionName
	}
	return strings.Join(p[0:lp], "/")
}

// processes an HttpSpanMessage
func (s *HTTPSpanMessage) process() {
	// always add to overall histogram
	recordHistogram(metricsHTTPHistograms, "", s.Duration)

	if s.Transaction != UnknownTransactionName {
		// only record the transaction-specific histogram and measurements if we are still within the limit
		// otherwise report it as an 'other' measurement
		if mTransMap.IsWithinLimit(s.Transaction) {
			recordHistogram(metricsHTTPHistograms, s.Transaction, s.Duration)
			s.processMeasurements(s.Transaction)
		} else {
			s.processMeasurements(OtherTransactionName)
		}
	} else {
		// no transaction/url name given, record as 'unknown'
		s.processMeasurements(UnknownTransactionName)
	}
}

// processes HTTP measurements, record one for primary key, and one for each secondary key
// transactionName	the transaction name to be used for these measurements
func (s *HTTPSpanMessage) processMeasurements(transactionName string) {
	name := "TransactionResponseTime"
	duration := float64(s.Duration)

	metricsHTTPMeasurements.lock.Lock()
	defer metricsHTTPMeasurements.lock.Unlock()

	// primary key: TransactionName
	primaryTags := make(map[string]string)
	primaryTags["TransactionName"] = transactionName
	recordMeasurement(metricsHTTPMeasurements, name, &primaryTags, duration, 1, true)

	// secondary keys: HttpMethod, HttpStatus, Errors
	withMethodTags := utils.CopyMap(&primaryTags)
	withMethodTags["HttpMethod"] = s.Method
	recordMeasurement(metricsHTTPMeasurements, name, &withMethodTags, duration, 1, true)

	withStatusTags := utils.CopyMap(&primaryTags)
	withStatusTags["HttpStatus"] = strconv.Itoa(s.Status)
	recordMeasurement(metricsHTTPMeasurements, name, &withStatusTags, duration, 1, true)

	if s.HasError {
		withErrorTags := utils.CopyMap(&primaryTags)
		withErrorTags["Errors"] = "true"
		recordMeasurement(metricsHTTPMeasurements, name, &withErrorTags, duration, 1, true)
	}
}

// records a measurement
// me			collection of measurements that this measurement should be added to
// name			key name
// tags			additional tags
// value		measurement value
// count		measurement count
// reportValue	should the sum of all values be reported?
func recordMeasurement(me *measurements, name string, tags *map[string]string,
	value float64, count int, reportValue bool) {

	measurements := me.measurements

	// assemble the ID for this measurement (a combination of different values)
	id := name + "&" + strconv.FormatBool(reportValue) + "&"

	// tags are part of the ID but since there's no guarantee that the map items
	// are always iterated in the same order, we need to sort them ourselves
	var tagsSorted []string
	for k, v := range *tags {
		tagsSorted = append(tagsSorted, k+":"+v)
	}
	sort.Strings(tagsSorted)

	// tags are all sorted now, append them to the ID
	for _, t := range tagsSorted {
		id += t + "&"
	}

	var m *Measurement
	var ok bool

	// create a new measurement if it doesn't exist
	if m, ok = measurements[id]; !ok {
		m = &Measurement{
			Name:      name,
			Tags:      *tags,
			ReportSum: reportValue,
		}
		measurements[id] = m
	}

	// add count and value
	m.Count += count
	m.Sum += value
}

// records a histogram
// hi		collection of histograms that this histogram should be added to
// name		key name
// duration	span duration
func recordHistogram(hi *histograms, name string, duration time.Duration) {
	hi.lock.Lock()
	defer func() {
		hi.lock.Unlock()
		if err := recover(); err != nil {
			log.Errorf("Failed to record histogram: %v", err)
		}
	}()

	histograms := hi.histograms
	id := name

	tags := make(map[string]string)
	if name != "" {
		tags["TransactionName"] = name
	}

	var h *histogram
	var ok bool

	// create a new histogram if it doesn't exist
	if h, ok = histograms[id]; !ok {
		h = &histogram{
			hist: hdrhist.WithConfig(hdrhist.Config{
				LowestDiscernible: 1,
				HighestTrackable:  3600000000,
				SigFigs:           int32(hi.precision),
			}),
			tags: tags,
		}
		histograms[id] = h
	}

	// record histogram
	h.hist.Record(int64(duration / time.Microsecond))
}

// adds a measurement to a BSON buffer
// bbuf		the BSON buffer to append the metric to
// index	a running integer (0,1,2,...) which is needed for BSON arrays
// m		measurement to be added
func addMeasurementToBSON(bbuf *bsonBuffer, index *int, m *Measurement) {
	start := bsonAppendStartObject(bbuf, strconv.Itoa(*index))

	bsonAppendString(bbuf, "name", m.Name)
	bsonAppendInt(bbuf, "count", m.Count)
	if m.ReportSum {
		bsonAppendFloat64(bbuf, "sum", m.Sum)
	}

	if len(m.Tags) > 0 {
		start := bsonAppendStartObject(bbuf, "tags")
		for k, v := range m.Tags {
			if len(k) > metricsTagNameLenghtMax {
				k = k[0:metricsTagNameLenghtMax]
			}
			if len(v) > metricsTagValueLenghtMax {
				v = v[0:metricsTagValueLenghtMax]
			}
			bsonAppendString(bbuf, k, v)
		}
		bsonAppendFinishObject(bbuf, start)
	}

	bsonAppendFinishObject(bbuf, start)
	*index += 1
}

// adds a histogram to a BSON buffer
// bbuf		the BSON buffer to append the metric to
// index	a running integer (0,1,2,...) which is needed for BSON arrays
// h		histogram to be added
func addHistogramToBSON(bbuf *bsonBuffer, index *int, h *histogram) {
	// get 64-base encoded representation of the histogram
	data, err := hdrhist.EncodeCompressed(h.hist)
	if err != nil {
		log.Errorf("Failed to encode histogram: %v", err)
		return
	}

	start := bsonAppendStartObject(bbuf, strconv.Itoa(*index))

	bsonAppendString(bbuf, "name", "TransactionResponseTime")
	bsonAppendString(bbuf, "value", string(data))

	// append tags
	if len(h.tags) > 0 {
		start := bsonAppendStartObject(bbuf, "tags")
		for k, v := range h.tags {
			if len(k) > metricsTagNameLenghtMax {
				k = k[0:metricsTagNameLenghtMax]
			}
			if len(v) > metricsTagValueLenghtMax {
				v = v[0:metricsTagValueLenghtMax]
			}
			bsonAppendString(bbuf, k, v)
		}
		bsonAppendFinishObject(bbuf, start)
	}

	bsonAppendFinishObject(bbuf, start)
	*index += 1
}

func (s *eventQueueStats) setQueueLargest(count int) {
	newVal := int64(count)

	for {
		currVal := atomic.LoadInt64(&s.queueLargest)
		if newVal <= currVal {
			return
		}
		if atomic.CompareAndSwapInt64(&s.queueLargest, currVal, newVal) {
			return
		}
	}
}

// copyAndReset returns a copy of its current values and reset itself.
func (s *eventQueueStats) copyAndReset() eventQueueStats {
	c := eventQueueStats{}

	c.numSent = atomic.SwapInt64(&s.numSent, 0)
	c.numFailed = atomic.SwapInt64(&s.numFailed, 0)
	c.totalEvents = atomic.SwapInt64(&s.totalEvents, 0)
	c.numOverflowed = atomic.SwapInt64(&s.numOverflowed, 0)
	c.queueLargest = atomic.SwapInt64(&s.queueLargest, 0)

	return c
}
