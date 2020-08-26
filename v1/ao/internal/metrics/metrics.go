// Copyright (C) 2017 Librato, Inc. All rights reserved.

package metrics

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/bson"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/hdrhist"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/host"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/pkg/errors"
)

const (
	metricsTransactionsMaxDefault = 200 // default max amount of transaction names we allow per cycle
	metricsHistPrecisionDefault   = 2   // default histogram precision

	metricsTagNameLengthMax  = 64  // max number of characters for tag names
	metricsTagValueLengthMax = 255 // max number of characters for tag values

	// MaxTagsCount is the maximum number of tags allowed
	MaxTagsCount = 50
)

// Special transaction names
const (
	UnknownTransactionName       = "unknown"
	OtherTransactionName         = "other"
	MetricIDSeparator            = "&"
	TagsKVSeparator              = ":"
	OtherMetricIDPrefix          = OtherTransactionName + MetricIDSeparator
	maxPathLenForTransactionName = 3
)

// Request counters definition
const (
	RequestCount               = "RequestCount"
	TraceCount                 = "TraceCount"
	TokenBucketExhaustionCount = "TokenBucketExhaustionCount"
	SampleCount                = "SampleCount"
	ThroughTraceCount          = "ThroughTraceCount"
	TriggeredTraceCount        = "TriggeredTraceCount"
)

// Request counters collection categories
const (
	RCRegular             = "ReqCounterRegular"
	RCRelaxedTriggerTrace = "ReqCounterRelaxedTriggerTrace"
	RCStrictTriggerTrace  = "ReqCounterStrictTriggerTrace"
)

var (
	// ErrExceedsMetricsCountLimit indicates there are too many distinct metrics.
	ErrExceedsMetricsCountLimit = errors.New("exceeds metrics count limit per flush interval")
	// ErrExceedsTagsCountLimit indicates there are too many tags
	ErrExceedsTagsCountLimit = errors.New("exceeds tags count limit")
	// ErrMetricsWithNonPositiveCount indicates the count is negative or zero
	ErrMetricsWithNonPositiveCount = errors.New("metrics with non-positive count")
)

// SpanMessage defines a span message
type SpanMessage interface {
	Process(m *Measurements)
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
	Tags      map[string]string // map of KVs. It may be nil
	Count     int               // count of this measurement
	Sum       float64           // sum for this measurement
	ReportSum bool              // include the sum in the report?
}

// Measurements are a collection of mutex-protected measurements
type Measurements struct {
	m             map[string]*Measurement
	transMap      *TransMap
	IsCustom      bool
	FlushInterval int32
	sync.Mutex    // protect access to this collection
}

func NewMeasurements(isCustom bool, flushInterval int32, maxCount int32) *Measurements {
	return &Measurements{
		m:             make(map[string]*Measurement),
		transMap:      NewTransMap(maxCount),
		IsCustom:      isCustom,
		FlushInterval: flushInterval,
	}
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

// EventQueueStats is the counters of the event queue stats
// All the fields are supposed to be accessed through atomic operations
type EventQueueStats struct {
	numSent       int64 // number of messages that were successfully sent
	numOverflowed int64 // number of messages that overflowed the queue
	numFailed     int64 // number of messages that failed to send
	totalEvents   int64 // number of messages queued to send
	queueLargest  int64 // maximum number of messages that were in the queue at one time
}

func (s *EventQueueStats) NumSentAdd(n int64) {
	atomic.AddInt64(&s.numSent, n)
}

func (s *EventQueueStats) NumOverflowedAdd(n int64) {
	atomic.AddInt64(&s.numOverflowed, n)
}

func (s *EventQueueStats) NumFailedAdd(n int64) {
	atomic.AddInt64(&s.numFailed, n)
}

func (s *EventQueueStats) TotalEventsAdd(n int64) {
	atomic.AddInt64(&s.totalEvents, n)
}

// RateCounts is the rate counts reported by trace sampler
type RateCounts struct{ requested, sampled, limited, traced, through int64 }

// FlushRateCounts reset the counters and returns the current value
func (c *RateCounts) FlushRateCounts() *RateCounts {
	return &RateCounts{
		requested: atomic.SwapInt64(&c.requested, 0),
		sampled:   atomic.SwapInt64(&c.sampled, 0),
		limited:   atomic.SwapInt64(&c.limited, 0),
		traced:    atomic.SwapInt64(&c.traced, 0),
		through:   atomic.SwapInt64(&c.through, 0),
	}
}

func (c *RateCounts) RequestedInc() {
	atomic.AddInt64(&c.requested, 1)
}

func (c *RateCounts) Requested() int64 {
	return atomic.LoadInt64(&c.requested)
}

func (c *RateCounts) SampledInc() {
	atomic.AddInt64(&c.sampled, 1)
}

func (c *RateCounts) Sampled() int64 {
	return atomic.LoadInt64(&c.sampled)
}

func (c *RateCounts) LimitedInc() {
	atomic.AddInt64(&c.limited, 1)
}

func (c *RateCounts) Limited() int64 {
	return atomic.LoadInt64(&c.limited)
}

func (c *RateCounts) TracedInc() {
	atomic.AddInt64(&c.traced, 1)
}

func (c *RateCounts) Traced() int64 {
	return atomic.LoadInt64(&c.traced)
}

func (c *RateCounts) ThroughInc() {
	atomic.AddInt64(&c.through, 1)
}

func (c *RateCounts) Through() int64 {
	return atomic.LoadInt64(&c.through)
}

// TransMap records the received transaction names in a metrics report cycle. It will refuse
// new transaction names if reaching the capacity.
type TransMap struct {
	// The map to store transaction names
	transactionNames map[string]struct{}
	// The maximum capacity of the transaction map. The value is got from server settings which
	// is updated periodically.
	// The default value metricsTransactionsMaxDefault is used when a new TransMap
	// is initialized.
	currCap int32
	// The maximum capacity which is set by the server settings. This update usually happens in
	// between two metrics reporting cycles. To avoid affecting the map capacity of the current reporting
	// cycle, the new capacity got from the server is stored in nextCap and will only be flushed to currCap
	// when the Reset() is called.
	nextCap int32
	// Whether there is an overflow. Overflow means the user tried to store more transaction names
	// than the capacity defined by settings.
	// This flag is cleared in every metrics cycle.
	overflow bool
	// The mutex to protect this whole struct. If the performance is a concern we should use separate
	// mutexes for each of the fields. But for now it seems not necessary.
	mutex sync.Mutex
}

// NewTransMap initializes a new TransMap struct
func NewTransMap(cap int32) *TransMap {
	return &TransMap{
		transactionNames: make(map[string]struct{}),
		currCap:          cap,
		nextCap:          cap,
		overflow:         false,
	}
}

// SetCap sets the capacity of the transaction map
func (t *TransMap) SetCap(cap int32) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.nextCap = cap
}

// Cap returns the current capacity
func (t *TransMap) Cap() int32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.currCap
}

// Reset resets the transaction map to a initialized state. The new capacity got from the
// server will be used in next metrics reporting cycle after reset.
func (t *TransMap) Reset() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.transactionNames = make(map[string]struct{})
	t.currCap = t.nextCap
	t.overflow = false
}

// Clone returns a shallow copy
func (t *TransMap) Clone() *TransMap {
	return &TransMap{
		transactionNames: t.transactionNames,
		currCap:          t.currCap,
		nextCap:          t.nextCap,
		overflow:         t.overflow,
	}
}

// IsWithinLimit checks if the transaction name is stored in the TransMap. It will store this new
// transaction name and return true if not stored before and the map isn't full, or return false
// otherwise.
func (t *TransMap) IsWithinLimit(name string) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, ok := t.transactionNames[name]; !ok {
		// only record if we haven't reached the limits yet
		if int32(len(t.transactionNames)) < t.currCap {
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

// addRequestCounters add various request-related counters to the metrics message buffer.
func addRequestCounters(bbuf *bson.Buffer, index *int, rcs map[string]*RateCounts) {
	var requested, traced, limited, ttTraced int64

	for _, rc := range rcs {
		requested += rc.Requested()
		traced += rc.Traced()
		limited += rc.Limited()
	}

	addMetricsValue(bbuf, index, RequestCount, requested)
	addMetricsValue(bbuf, index, TraceCount, traced)
	addMetricsValue(bbuf, index, TokenBucketExhaustionCount, limited)

	if rcRegular, ok := rcs[RCRegular]; ok {
		addMetricsValue(bbuf, index, SampleCount, rcRegular.Sampled())
		addMetricsValue(bbuf, index, ThroughTraceCount, rcRegular.Through())
	}

	if relaxed, ok := rcs[RCRelaxedTriggerTrace]; ok {
		ttTraced += relaxed.Traced()
	}
	if strict, ok := rcs[RCStrictTriggerTrace]; ok {
		ttTraced += strict.Traced()
	}

	addMetricsValue(bbuf, index, TriggeredTraceCount, ttTraced)
}

// BuildMessage creates and encodes the custom metrics message.
func BuildMessage(m *Measurements, serverless bool) []byte {
	if m == nil {
		return nil
	}

	bbuf := bson.NewBuffer()
	if m.IsCustom {
		bbuf.AppendBool("IsCustom", m.IsCustom)
	}

	if !serverless {
		appendHostId(bbuf)
		bbuf.AppendInt32("MetricsFlushInterval", m.FlushInterval)
	}

	bbuf.AppendInt64("Timestamp_u", time.Now().UnixNano()/1000)

	start := bbuf.AppendStartArray("measurements")
	index := 0

	for _, measurement := range m.m {
		addMeasurementToBSON(bbuf, &index, measurement)
	}

	bbuf.AppendFinishObject(start)

	bbuf.Finish()
	return bbuf.GetBuf()
}

// SetCap sets the maximum number of distinct metrics allowed.
func (m *Measurements) SetCap(cap int32) {
	m.transMap.SetCap(cap)
}

// Cap returns the maximum number of distinct metrics allowed.
func (m *Measurements) Cap() int32 {
	return m.transMap.Cap()
}

// CopyAndReset resets the custom metrics and return a copy of the old one.
func (m *Measurements) CopyAndReset(flushInterval int32) *Measurements {
	m.Lock()
	defer m.Unlock()

	if len(m.m) == 0 {
		m.FlushInterval = flushInterval
		return nil
	}

	clone := m.Clone()
	m.m = make(map[string]*Measurement)
	m.transMap.Reset()
	m.FlushInterval = flushInterval
	return clone
}

// Clone returns a shallow copy
func (m *Measurements) Clone() *Measurements {
	return &Measurements{
		m:             m.m,
		transMap:      m.transMap.Clone(),
		IsCustom:      m.IsCustom,
		FlushInterval: m.FlushInterval,
	}
}

// Summary submits the summary measurement to the reporter.
func (m *Measurements) Summary(name string, value float64, opts MetricOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}
	return m.recordWithSoloTags(name, opts.Tags, value, opts.Count, true)
}

// Increment submits the incremental measurement to the reporter.
func (m *Measurements) Increment(name string, opts MetricOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}
	return m.recordWithSoloTags(name, opts.Tags, 0, opts.Count, false)
}

// MetricOptions is a struct for the optional parameters of a measurement.
type MetricOptions struct {
	Count   int
	HostTag bool
	Tags    map[string]string
}

func (mo *MetricOptions) validate() error {
	if len(mo.Tags) > MaxTagsCount {
		return ErrExceedsTagsCountLimit
	}

	if mo.Count <= 0 {
		return ErrMetricsWithNonPositiveCount
	}

	return nil
}

func addRuntimeMetrics(bbuf *bson.Buffer, index *int) {
	// category runtime
	addMetricsValue(bbuf, index, "trace.go.runtime.NumGoroutine", runtime.NumGoroutine())
	addMetricsValue(bbuf, index, "trace.go.runtime.NumCgoCall", runtime.NumCgoCall())

	var mem runtime.MemStats
	host.Mem(&mem)
	// category gc
	addMetricsValue(bbuf, index, "trace.go.gc.LastGC", int64(mem.LastGC))
	addMetricsValue(bbuf, index, "trace.go.gc.NextGC", int64(mem.NextGC))
	addMetricsValue(bbuf, index, "trace.go.gc.PauseTotalNs", int64(mem.PauseTotalNs))
	addMetricsValue(bbuf, index, "trace.go.gc.NumGC", int64(mem.NumGC))
	addMetricsValue(bbuf, index, "trace.go.gc.NumForcedGC", int64(mem.NumForcedGC))
	addMetricsValue(bbuf, index, "trace.go.gc.GCCPUFraction", mem.GCCPUFraction)

	// category memory
	addMetricsValue(bbuf, index, "trace.go.memory.Alloc", int64(mem.Alloc))
	addMetricsValue(bbuf, index, "trace.go.memory.TotalAlloc", int64(mem.TotalAlloc))
	addMetricsValue(bbuf, index, "trace.go.memory.Sys", int64(mem.Sys))
	addMetricsValue(bbuf, index, "trace.go.memory.Lookups", int64(mem.Lookups))
	addMetricsValue(bbuf, index, "trace.go.memory.Mallocs", int64(mem.Mallocs))
	addMetricsValue(bbuf, index, "trace.go.memory.Frees", int64(mem.Frees))
	addMetricsValue(bbuf, index, "trace.go.memory.HeapAlloc", int64(mem.HeapAlloc))
	addMetricsValue(bbuf, index, "trace.go.memory.HeapSys", int64(mem.HeapSys))
	addMetricsValue(bbuf, index, "trace.go.memory.HeapIdle", int64(mem.HeapIdle))
	addMetricsValue(bbuf, index, "trace.go.memory.HeapInuse", int64(mem.HeapInuse))
	addMetricsValue(bbuf, index, "trace.go.memory.HeapReleased", int64(mem.HeapReleased))
	addMetricsValue(bbuf, index, "trace.go.memory.HeapObjects", int64(mem.HeapObjects))
	addMetricsValue(bbuf, index, "trace.go.memory.StackInuse", int64(mem.StackInuse))
	addMetricsValue(bbuf, index, "trace.go.memory.StackSys", int64(mem.StackSys))
}

// BuildBuiltinMetricsMessage generates a metrics message in BSON format with all the currently available values
// metricsFlushInterval	current metrics flush interval
//
// return				metrics message in BSON format
func BuildBuiltinMetricsMessage(m *Measurements, qs *EventQueueStats,
	rcs map[string]*RateCounts, runtimeMetrics bool) []byte {
	if m == nil {
		return nil
	}

	bbuf := bson.NewBuffer()

	appendHostId(bbuf)
	bbuf.AppendInt32("MetricsFlushInterval", m.FlushInterval)

	bbuf.AppendInt64("Timestamp_u", int64(time.Now().UnixNano()/1000))

	// measurements
	// ==========================================
	start := bbuf.AppendStartArray("measurements")
	index := 0

	// request counters
	addRequestCounters(bbuf, &index, rcs)

	// Queue states
	if qs != nil {
		addMetricsValue(bbuf, &index, "NumSent", qs.numSent)
		addMetricsValue(bbuf, &index, "NumOverflowed", qs.numOverflowed)
		addMetricsValue(bbuf, &index, "NumFailed", qs.numFailed)
		addMetricsValue(bbuf, &index, "TotalEvents", qs.totalEvents)
		addMetricsValue(bbuf, &index, "QueueLargest", qs.queueLargest)
	}

	addHostMetrics(bbuf, &index)

	if runtimeMetrics {
		// runtime stats
		addRuntimeMetrics(bbuf, &index)
	}

	for _, measurement := range m.m {
		addMeasurementToBSON(bbuf, &index, measurement)
	}

	bbuf.AppendFinishObject(start)
	// ==========================================

	// histograms
	// ==========================================
	start = bbuf.AppendStartArray("histograms")
	index = 0

	metricsHTTPHistograms.lock.Lock()

	for _, h := range metricsHTTPHistograms.histograms {
		addHistogramToBSON(bbuf, &index, h)
	}
	metricsHTTPHistograms.histograms = make(map[string]*histogram) // clear histograms

	metricsHTTPHistograms.lock.Unlock()
	bbuf.AppendFinishObject(start)
	// ==========================================

	if m.transMap.Overflow() {
		bbuf.AppendBool("TransactionNameOverflow", true)
	}

	bbuf.Finish()
	return bbuf.GetBuf()
}

// append host ID to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendHostId(bbuf *bson.Buffer) {
	if host.ConfiguredHostname() != "" {
		bbuf.AppendString("ConfiguredHostname", host.ConfiguredHostname())
	}
	appendUname(bbuf)
	bbuf.AppendString("Distro", host.Distro())
	appendIPAddresses(bbuf)
}

// gets and appends IP addresses to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendIPAddresses(bbuf *bson.Buffer) {
	addrs := host.IPAddresses()
	if addrs == nil {
		return
	}

	start := bbuf.AppendStartArray("IPAddresses")
	for i, address := range addrs {
		bbuf.AppendString(strconv.Itoa(i), address)
	}
	bbuf.AppendFinishObject(start)
}

// gets and appends MAC addresses to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendMACAddresses(bbuf *bson.Buffer, macs []string) {
	start := bbuf.AppendStartArray("MACAddresses")
	for _, mac := range macs {
		if mac == "" {
			continue
		}
		i := 0
		bbuf.AppendString(strconv.Itoa(i), mac)
		i++
	}
	bbuf.AppendFinishObject(start)
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
func addMetricsValue(bbuf *bson.Buffer, index *int, name string, value interface{}) {
	start := bbuf.AppendStartObject(strconv.Itoa(*index))
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("%v", err)
		}
	}()

	bbuf.AppendString("name", name)
	switch value.(type) {
	case int:
		bbuf.AppendInt("value", value.(int))
	case int64:
		bbuf.AppendInt64("value", value.(int64))
	case float32:
		v32 := value.(float32)
		v64 := float64(v32)
		bbuf.AppendFloat64("value", v64)
	case float64:
		bbuf.AppendFloat64("value", value.(float64))
	default:
		bbuf.AppendString("value", "unknown")
	}

	bbuf.AppendFinishObject(start)
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

// Process processes an HttpSpanMessage
func (s *HTTPSpanMessage) Process(m *Measurements) {
	// always add to overall histogram
	recordHistogram(metricsHTTPHistograms, "", s.Duration)

	if s.Transaction == UnknownTransactionName {
		s.processMeasurements(nil, m)
		return
	}

	// only record the transaction-specific histogram and measurements if we are still within the limit
	// otherwise report it as an 'other' measurement
	if err, reusableTags := s.processMeasurements(nil, m); err == ErrExceedsMetricsCountLimit {
		s.Transaction = OtherTransactionName
		s.processMeasurements(reusableTags, m)
		return
	}

	recordHistogram(metricsHTTPHistograms, s.Transaction, s.Duration)
}

func (s *HTTPSpanMessage) produceTagsList() []map[string]string {
	var tagsList []map[string]string

	// primary key: TransactionName
	primaryTags := make(map[string]string)
	primaryTags["TransactionName"] = s.Transaction
	tagsList = append(tagsList, primaryTags)

	// secondary keys: HttpMethod, HttpStatus, Errors
	withMethodTags := utils.CopyMap(&primaryTags)
	withMethodTags["HttpMethod"] = s.Method
	tagsList = append(tagsList, withMethodTags)

	withStatusTags := utils.CopyMap(&primaryTags)
	withStatusTags["HttpStatus"] = strconv.Itoa(s.Status)
	tagsList = append(tagsList, withStatusTags)

	if s.HasError {
		withErrorTags := utils.CopyMap(&primaryTags)
		withErrorTags["Errors"] = "true"
		tagsList = append(tagsList, withErrorTags)
	}

	return tagsList
}

// processes HTTP measurements, record one for primary key, and one for each secondary key
// transactionName	the transaction name to be used for these measurements
func (s *HTTPSpanMessage) processMeasurements(tagsList []map[string]string,
	m *Measurements) (error, []map[string]string) {
	name := "TransactionResponseTime"
	duration := float64(s.Duration / time.Microsecond)

	if tagsList == nil {
		tagsList = s.produceTagsList()
	}

	err := m.record(name, tagsList, duration, 1, true)

	if err != nil {
		return err, tagsList
	}
	return nil, nil
}

func (m *Measurements) recordWithSoloTags(name string, tags map[string]string,
	value float64, count int, reportValue bool) error {
	return m.record(name, []map[string]string{tags}, value, count, reportValue)
}

// records a measurement
// name			key name
// tagsList		the list of the additional tags
// value		measurement value
// count		measurement count
// reportValue	should the sum of all values be reported?
func (m *Measurements) record(name string, tagsList []map[string]string,
	value float64, count int, reportValue bool) error {
	if len(tagsList) == 0 {
		return nil
	}

	idTagsMap := make(map[string]map[string]string)
	idPrefixList := []string{name, strconv.FormatBool(reportValue)}

	for _, tags := range tagsList {
		idList := append(idPrefixList[:0:0], idPrefixList...)
		if tags != nil {
			// tags are part of the ID but since there's no guarantee that the map items
			// are always iterated in the same order, we need to sort them ourselves
			var tagsSorted []string
			for k, v := range tags {
				tagsSorted = append(tagsSorted, k+TagsKVSeparator+v)
			}
			sort.Strings(tagsSorted)

			idList = append(idList, tagsSorted...)
		}
		idList = append(idList, "")
		id := strings.Join(idList, MetricIDSeparator)

		idTagsMap[id] = tags
	}

	var me *Measurement
	var ok bool

	// create a new measurement if it doesn't exist
	// the lock protects both Measurements and Measurement
	m.Lock()
	defer m.Unlock()
	for id, tags := range idTagsMap {
		if me, ok = m.m[id]; !ok {
			if strings.HasPrefix(id, OtherMetricIDPrefix) ||
				m.transMap.IsWithinLimit(id) {
				me = &Measurement{
					Name:      name,
					Tags:      tags,
					ReportSum: reportValue,
				}
				m.m[id] = me
			} else {
				return ErrExceedsMetricsCountLimit
			}
		}

		// add count and value
		me.Count += count
		me.Sum += value
	}
	return nil
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
func addMeasurementToBSON(bbuf *bson.Buffer, index *int, m *Measurement) {
	start := bbuf.AppendStartObject(strconv.Itoa(*index))

	bbuf.AppendString("name", m.Name)
	bbuf.AppendInt("count", m.Count)
	if m.ReportSum {
		bbuf.AppendFloat64("sum", m.Sum)
	}

	if len(m.Tags) > 0 {
		start := bbuf.AppendStartObject("tags")
		for k, v := range m.Tags {
			if len(k) > metricsTagNameLengthMax {
				k = k[0:metricsTagNameLengthMax]
			}
			if len(v) > metricsTagValueLengthMax {
				v = v[0:metricsTagValueLengthMax]
			}
			bbuf.AppendString(k, v)
		}
		bbuf.AppendFinishObject(start)
	}

	bbuf.AppendFinishObject(start)
	*index += 1
}

// adds a histogram to a BSON buffer
// bbuf		the BSON buffer to append the metric to
// index	a running integer (0,1,2,...) which is needed for BSON arrays
// h		histogram to be added
func addHistogramToBSON(bbuf *bson.Buffer, index *int, h *histogram) {
	// get 64-base encoded representation of the histogram
	data, err := hdrhist.EncodeCompressed(h.hist)
	if err != nil {
		log.Errorf("Failed to encode histogram: %v", err)
		return
	}

	start := bbuf.AppendStartObject(strconv.Itoa(*index))

	bbuf.AppendString("name", "TransactionResponseTime")
	bbuf.AppendString("value", string(data))

	// append tags
	if len(h.tags) > 0 {
		start := bbuf.AppendStartObject("tags")
		for k, v := range h.tags {
			if len(k) > metricsTagNameLengthMax {
				k = k[0:metricsTagNameLengthMax]
			}
			if len(v) > metricsTagValueLengthMax {
				v = v[0:metricsTagValueLengthMax]
			}
			bbuf.AppendString(k, v)
		}
		bbuf.AppendFinishObject(start)
	}

	bbuf.AppendFinishObject(start)
	*index += 1
}

func (s *EventQueueStats) SetQueueLargest(count int64) {
	newVal := count

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

// CopyAndReset returns a copy of its current values and reset itself.
func (s *EventQueueStats) CopyAndReset() *EventQueueStats {
	c := &EventQueueStats{}

	c.numSent = atomic.SwapInt64(&s.numSent, 0)
	c.numFailed = atomic.SwapInt64(&s.numFailed, 0)
	c.totalEvents = atomic.SwapInt64(&s.totalEvents, 0)
	c.numOverflowed = atomic.SwapInt64(&s.numOverflowed, 0)
	c.queueLargest = atomic.SwapInt64(&s.queueLargest, 0)

	return c
}

func BuildServerlessMessage(span HTTPSpanMessage) []byte {
	bbuf := bson.NewBuffer()

	bbuf.AppendInt64("Duration", int64(span.Duration/time.Microsecond))
	bbuf.AppendBool("HasError", span.HasError)
	bbuf.AppendBool("IsHTTPSpan", span.Method != "")
	bbuf.AppendString("Method", span.Method)
	bbuf.AppendInt("Status", span.Status)
	bbuf.AppendInt64("Timestamp_u", time.Now().UnixNano()/1000)
	bbuf.AppendString("TransactionName", span.Transaction)

	bbuf.Finish()
	return bbuf.GetBuf()
}
