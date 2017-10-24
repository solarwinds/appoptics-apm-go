// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/librato/go-traceview/v1/tv/internal/hdrhist"
)

// Linux distributions and their identifying files
const (
	REDHAT    = "/etc/redhat-release"
	AMAZON    = "/etc/release-cpe"
	UBUNTU    = "/etc/lsb-release"
	DEBIAN    = "/etc/debian_version"
	SUSE      = "/etc/SuSE-release"
	SLACKWARE = "/etc/slackware-version"
	GENTOO    = "/etc/gentoo-release"
	OTHER     = "/etc/issue"
)

const (
	metricsTransactionsMaxDefault = 200 // default max amount of transaction names we allow per cycle
	metricsHistPrecisionDefault   = 2   // default histogram precision

	metricsTagNameLenghtMax  = 64  // max number of characters for tag names
	metricsTagValueLenghtMax = 255 // max number of characters for tag values
)

// defines a span message
type SpanMessage interface {
	// called for message processing
	process()
}

// base span message with properties found in all types of span messages
type BaseSpanMessage struct {
	Duration time.Duration // duration of the span (nanoseconds)
	HasError bool          // boolean flag whether this transaction contains an error or not
}

// HTTP span message used for inbound metrics
type HttpSpanMessage struct {
	BaseSpanMessage
	Transaction string // transaction name (e.g. controller.action)
	Url         string // the raw url which will be processed and used as transaction (if Transaction is empty)
	Status      int    // HTTP status code (e.g. 200, 500, ...)
	Method      string // HTTP method (e.g. GET, POST, ...)
}

// a single measurement
type measurement struct {
	name      string            // the name of the measurement (e.g. TransactionResponseTime)
	tags      map[string]string // map of KVs
	count     int               // count of this measurement
	sum       float64           // sum for this measurement
	reportSum bool              // include the sum in the report?
}

// a collection of measurements
type measurements struct {
	measurements            map[string]*measurement
	transactionNameMax      int            // max transaction names
	transactionNameOverflow bool           // have we hit the limit of allowable transaction names?
	uRLRegex                *regexp.Regexp // regular expression used for URL fingerprinting
	lock                    sync.Mutex     // protect access to this collection
	transactionNameMaxLock  sync.RWMutex   // lock to ensure sequential access
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
type eventQueueStats struct {
	numSent       int64      // number of messages that were successfully sent
	numOverflowed int64      // number of messages that overflowed the queue
	numFailed     int64      // number of messages that failed to send
	totalEvents   int64      // number of messages queued to send
	queueLargest  int64      // maximum number of messages that were in the queue at one time
	lock          sync.Mutex // protect access to the counters
}

var cachedDistro string                     // cached distribution name
var cachedMACAddresses = "uninitialized"    // cached list MAC addresses
var cachedAWSInstanceId = "uninitialized"   // cached EC2 instance ID (if applicable)
var cachedAWSInstanceZone = "uninitialized" // cached EC2 instance zone (if applicable)
var cachedContainerID = "uninitialized"     // cached docker container ID (if applicable)

// list of currently stored unique HTTP transaction names (flushed on each metrics report cycle)
var metricsHTTPTransactions = make(map[string]bool)

// collection of currently stored measurements (flushed on each metrics report cycle)
var metricsHTTPMeasurements = &measurements{
	measurements:       make(map[string]*measurement),
	transactionNameMax: metricsTransactionsMaxDefault,
	uRLRegex:           regexp.MustCompile(`^(https?://)?[^/]+(/([^/\?]+))?(/([^/\?]+))?`),
}

// collection of currently stored histograms (flushed on each metrics report cycle)
var metricsHTTPHistograms = &histograms{
	histograms: make(map[string]*histogram),
	precision:  metricsHistPrecisionDefault,
}

// event queue stats (reset on each metrics report cycle)
var metricsEventQueueStats = &eventQueueStats{}

// initialize values according to env variables
func init() {
	pEnv := "APPOPTICS_HISTOGRAM_PRECISION"
	precision := os.Getenv(pEnv)
	if precision != "" {
		if p, err := strconv.Atoi(precision); err == nil {
			if p >= 0 && p <= 5 {
				metricsHTTPHistograms.precision = p
			} else {
				OboeLog(ERROR, fmt.Sprintf(
					"value of %v must be between 0 and 5: %v", pEnv, precision))
			}
		} else {
			OboeLog(ERROR, fmt.Sprintf(
				"value of %v is not an int: %v", pEnv, precision))
		}
	}
}

// generates a metrics message in BSON format with all the currently available values
// metricsFlushInterval	current metrics flush interval
//
// return				metrics message in BSON format
func generateMetricsMessage(metricsFlushInterval int) []byte {
	bbuf := NewBsonBuffer()

	bsonAppendString(bbuf, "Hostname", cachedHostname)
	bsonAppendString(bbuf, "Distro", getDistro())
	bsonAppendInt(bbuf, "PID", cachedPid)
	appendUname(bbuf)
	appendIPAddresses(bbuf)
	appendMACAddresses(bbuf)

	if getAWSInstanceID() != "" {
		bsonAppendString(bbuf, "EC2InstanceID", getAWSInstanceID())
	}
	if getAWSInstanceZone() != "" {
		bsonAppendString(bbuf, "EC2AvailabilityZone", getAWSInstanceZone())
	}
	if getContainerId() != "" {
		bsonAppendString(bbuf, "DockerContainerID", getContainerId())
	}

	bsonAppendInt64(bbuf, "Timestamp_u", int64(time.Now().UnixNano()/1000))
	bsonAppendInt(bbuf, "MetricsFlushInterval", metricsFlushInterval)

	// measurements
	// ==========================================
	start := bsonAppendStartArray(bbuf, "measurements")
	index := 0

	// TODO add request counters

	// event queue stats
	metricsEventQueueStats.lock.Lock()

	addMetricsValue(bbuf, &index, "NumSent", metricsEventQueueStats.numSent)
	addMetricsValue(bbuf, &index, "NumOverflowed", metricsEventQueueStats.numOverflowed)
	addMetricsValue(bbuf, &index, "NumFailed", metricsEventQueueStats.numFailed)
	addMetricsValue(bbuf, &index, "TotalEvents", metricsEventQueueStats.totalEvents)
	addMetricsValue(bbuf, &index, "QueueLargest", metricsEventQueueStats.queueLargest)

	metricsEventQueueStats.numSent = 0
	metricsEventQueueStats.numOverflowed = 0
	metricsEventQueueStats.numFailed = 0
	metricsEventQueueStats.totalEvents = 0
	metricsEventQueueStats.queueLargest = 0

	metricsEventQueueStats.lock.Unlock()

	// system load of last minute
	if s := getStrByKeyword("/proc/loadavg", ""); s != "" {
		load, err := strconv.ParseFloat(strings.Fields(s)[0], 64)
		if err == nil {
			addMetricsValue(bbuf, &index, "Load1", load)
		}
	}

	// system total memory
	if s := getStrByKeyword("/proc/meminfo", "MemTotal"); s != "" {
		memTotal := strings.Fields(s) // MemTotal: 7657668 kB
		if len(memTotal) == 3 {
			if total, err := strconv.Atoi(memTotal[1]); err == nil {
				addMetricsValue(bbuf, &index, "TotalRAM", total*1024)
			}
		}
	}

	// free memory
	if s := getStrByKeyword("/proc/meminfo", "MemFree"); s != "" {
		memFree := strings.Fields(s) // MemFree: 161396 kB
		if len(memFree) == 3 {
			if free, err := strconv.Atoi(memFree[1]); err == nil {
				addMetricsValue(bbuf, &index, "FreeRAM", free*1024) // bytes
			}
		}
	}

	// process memory
	if s := getStrByKeyword("/proc/self/statm", ""); s != "" {
		processRAM := strings.Fields(s)
		if len(processRAM) != 0 {
			for _, ps := range processRAM {
				if p, err := strconv.Atoi(ps); err == nil {
					addMetricsValue(bbuf, &index, "ProcessRAM", p*os.Getpagesize())
					break
				}
			}
		}
	}

	// runtime stats
	addMetricsValue(bbuf, &index, "JMX.type=threadcount,name=NumGoroutine", runtime.NumGoroutine())
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
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
	debug.ReadGCStats(&gc)
	addMetricsValue(bbuf, &index, "JMX.type=count,name=GCStats.NumGC", gc.NumGC)

	// service / transaction measurements
	metricsHTTPMeasurements.lock.Lock()
	transactionNameOverflow := metricsHTTPMeasurements.transactionNameOverflow

	for _, m := range metricsHTTPMeasurements.measurements {
		addMeasurementToBSON(bbuf, &index, m)
	}
	metricsHTTPMeasurements.measurements = make(map[string]*measurement)
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
	metricsHTTPHistograms.histograms = make(map[string]*histogram)

	metricsHTTPHistograms.lock.Unlock()
	bsonAppendFinishObject(bbuf, start)
	// ==========================================

	if transactionNameOverflow {
		bsonAppendBool(bbuf, "TransactionNameOverflow", true)
	}

	bsonBufferFinish(bbuf)
	return bbuf.buf
}

// gets distribution identification
func getDistro() string {
	if cachedDistro != "" {
		return cachedDistro
	}

	var ds []string // distro slice

	// Note: Order of checking is important because some distros share same file names
	// but with different function.
	// Keep this order: redhat based -> ubuntu -> debian

	// redhat
	if cachedDistro = getStrByKeyword(REDHAT, ""); cachedDistro != "" {
		return cachedDistro
	}
	// amazon linux
	cachedDistro = getStrByKeyword(AMAZON, "")
	ds = strings.Split(cachedDistro, ":")
	cachedDistro = ds[len(ds)-1]
	if cachedDistro != "" {
		cachedDistro = "Amzn Linux " + cachedDistro
		return cachedDistro
	}
	// ubuntu
	cachedDistro = getStrByKeyword(UBUNTU, "DISTRIB_DESCRIPTION")
	if cachedDistro != "" {
		ds = strings.Split(cachedDistro, "=")
		cachedDistro = ds[len(ds)-1]
		if cachedDistro != "" {
			cachedDistro = strings.Trim(cachedDistro, "\"")
		} else {
			cachedDistro = "Ubuntu unknown"
		}
		return cachedDistro
	}

	pathes := []string{DEBIAN, SUSE, SLACKWARE, GENTOO, OTHER}
	if path, line := getStrByKeywordFiles(pathes, ""); path != "" && line != "" {
		cachedDistro = line
		if path == "Debian" {
			cachedDistro = "Debian " + cachedDistro
		}
	} else {
		cachedDistro = "Unknown"
	}
	return cachedDistro
}

// gets and appends UnameSysName/UnameVersion to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendUname(bbuf *bsonBuffer) {
	if runtime.GOOS == "linux" {
		var uname syscall.Utsname
		if err := syscall.Uname(&uname); err == nil {
			sysname := Byte2String(uname.Sysname[:])
			version := Byte2String(uname.Version[:])
			bsonAppendString(bbuf, "UnameSysName", strings.TrimRight(sysname, "\x00"))
			bsonAppendString(bbuf, "UnameVersion", strings.TrimRight(version, "\x00"))
		}
	}
}

// gets and appends IP addresses to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendIPAddresses(bbuf *bsonBuffer) {
	addrs := getIPAddresses()
	if addrs == nil {
		return
	}

	i := 0
	start := bsonAppendStartArray(bbuf, "IPAddresses")
	for _, address := range addrs {
		bsonAppendString(bbuf, strconv.Itoa(i), address)
		i++
	}
	bsonAppendFinishObject(bbuf, start)
}

// gets the system's IP addresses
func getIPAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	var addresses []string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			addresses = append(addresses, ipnet.IP.String())
		}
	}

	return addresses
}

// gets and appends MAC addresses to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendMACAddresses(bbuf *bsonBuffer) {
	macs := strings.Split(getMACAddressList(), ",")

	start := bsonAppendStartArray(bbuf, "MACAddresses")
	for _, mac := range macs {
		i := 0
		bsonAppendString(bbuf, strconv.Itoa(i), mac)
		i++
	}
	bsonAppendFinishObject(bbuf, start)
}

// gets a comma-separated list of MAC addresses
func getMACAddressList() string {
	if cachedMACAddresses != "uninitialized" {
		return cachedMACAddresses
	}

	cachedMACAddresses = ""
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if mac := iface.HardwareAddr.String(); mac != "" {
				cachedMACAddresses += iface.HardwareAddr.String() + ","
			}
		}
	}
	cachedMACAddresses = strings.TrimSuffix(cachedMACAddresses, ",") // trim the final one

	return cachedMACAddresses
}

// gets the AWS instance ID (or empty string if not an AWS instance)
func getAWSInstanceID() string {
	if cachedAWSInstanceId != "uninitialized" {
		return cachedAWSInstanceId
	}

	cachedAWSInstanceId = ""
	if isEC2Instance() {
		url := "http://169.254.169.254/latest/meta-data/instance-id"
		client := http.Client{Timeout: time.Second}
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				cachedAWSInstanceId = string(body)
			}
		}
	}

	return cachedAWSInstanceId
}

// gets the AWS instance zone (or empty string if not an AWS instance)
func getAWSInstanceZone() string {
	if cachedAWSInstanceZone != "uninitialized" {
		return cachedAWSInstanceZone
	}

	cachedAWSInstanceZone = ""
	if isEC2Instance() {
		url := "http://169.254.169.254/latest/meta-data/placement/availability-zone"
		client := http.Client{Timeout: time.Second}
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				cachedAWSInstanceZone = string(body)
			}
		}
	}

	return cachedAWSInstanceZone
}

// check if this an EC2 instance
func isEC2Instance() bool {
	match := getLineByKeyword("/sys/hypervisor/uuid", "ec2")
	return match != "" && strings.HasPrefix(match, "ec2")
}

// gets the docker container ID (or empty string if not a docker container)
func getContainerId() string {
	if cachedContainerID != "uninitialized" {
		return cachedContainerID
	}

	cachedContainerID = ""
	line := getLineByKeyword("/proc/self/cgroup", "docker")
	if line != "" {
		tokens := strings.Split(line, "/")
		// A typical line returned by cat /proc/self/cgroup (that's why we expect 3 tokens):
		// 9:devices:/docker/40188af19439697187e3f60b933e7e37c5c41035f4c0b266a51c86c5a0074b25
		if len(tokens) == 3 {
			cachedContainerID = tokens[2]
		}
	}

	return cachedContainerID
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

	bsonAppendString(bbuf, "name", name)
	switch value.(type) {
	case int:
		bsonAppendInt(bbuf, "value", value.(int))
	case int64:
		bsonAppendInt64(bbuf, "value", value.(int64))
	case float32, float64:
		bsonAppendFloat64(bbuf, "value", value.(float64))
	default:
		bsonAppendString(bbuf, "value", "unknown")
	}

	bsonAppendFinishObject(bbuf, start)
	*index += 1
}

// performs URL fingerprinting on a given URL to extract the transaction name
// e.g. https://github.com/librato/go-traceview/blob/metrics becomes /librato/go-traceview
func getTransactionFromURL(url string) string {
	matches := metricsHTTPMeasurements.uRLRegex.FindStringSubmatch(url)
	var ret string
	if matches[3] != "" {
		ret += "/" + matches[3]
		if matches[5] != "" {
			ret += "/" + matches[5]
		}
	} else {
		ret = "/"
	}

	return ret
}

// check if an element is found in a list, add if the list limit hasn't been reached yet
// m		list of elements
// element	element to look up or add
// max		max allowable elements in the list
//
// return	true if element has been found or has been added to the list successfully, false otherwise
func isWithinLimit(m *map[string]bool, element string, max int) bool {
	if _, ok := (*m)[element]; !ok {
		// only record if we haven't reached the limits yet
		if len(*m) < max {
			(*m)[element] = true
			return true
		}
		return false
	} else {
		return true
	}
}

// processes an HttpSpanMessage
func (httpSpan *HttpSpanMessage) process() {
	// always add to overall histogram
	recordHistogram(metricsHTTPHistograms, "", httpSpan.Duration)

	// check if we need to perform URL fingerprinting (no transaction name passed in)
	if httpSpan.Transaction == "" && httpSpan.Url != "" {
		httpSpan.Transaction = getTransactionFromURL(httpSpan.Url)
	}
	if httpSpan.Transaction != "" {
		// access transactionNameMax protected since it can be updated in updateSettings()
		metricsHTTPMeasurements.transactionNameMaxLock.RLock()
		max := metricsHTTPMeasurements.transactionNameMax
		metricsHTTPMeasurements.transactionNameMaxLock.RUnlock()

		transactionWithinLimit := isWithinLimit(
			&metricsHTTPTransactions, httpSpan.Transaction, max)

		// only record the transaction-specific histogram and measurements if we are still within the limit
		// otherwise report it as an 'other' measurement
		if transactionWithinLimit {
			recordHistogram(metricsHTTPHistograms, httpSpan.Transaction, httpSpan.Duration)
			httpSpan.processMeasurements(httpSpan.Transaction)
		} else {
			httpSpan.processMeasurements("other")
			// indicate we have overrun the transaction name limit
			setTransactionNameOverflow(true)
		}
	} else {
		// no transaction/url name given, record as 'unknown'
		httpSpan.processMeasurements("unknown")
	}
}

// processes HTTP measurements, record one for primary key, and one for each secondary key
// transactionName	the transaction name to be used for these measurements
func (httpSpan *HttpSpanMessage) processMeasurements(transactionName string) {
	name := "TransactionResponseTime"
	duration := float64((*httpSpan).Duration)

	metricsHTTPMeasurements.lock.Lock()
	defer metricsHTTPMeasurements.lock.Unlock()

	// primary key: TransactionName
	primaryTags := make(map[string]string)
	primaryTags["TransactionName"] = transactionName
	recordMeasurement(metricsHTTPMeasurements, name, &primaryTags, duration, 1, true)

	// secondary keys: HttpMethod, HttpStatus, Errors
	withMethodTags := copyMap(&primaryTags)
	withMethodTags["HttpMethod"] = httpSpan.Method
	recordMeasurement(metricsHTTPMeasurements, name, &withMethodTags, duration, 1, true)

	withStatusTags := copyMap(&primaryTags)
	withStatusTags["HttpStatus"] = strconv.Itoa(httpSpan.Status)
	recordMeasurement(metricsHTTPMeasurements, name, &withStatusTags, duration, 1, true)

	if httpSpan.HasError {
		withErrorTags := copyMap(&primaryTags)
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
	for k, v := range *tags {
		id += k + ":" + v + "&"
	}

	var m *measurement
	var ok bool

	// create a new measurement if it doesn't exist
	if m, ok = measurements[id]; !ok {
		m = &measurement{
			name:      name,
			tags:      *tags,
			reportSum: reportValue,
		}
		measurements[id] = m
	}

	// add count and value
	m.count += count
	m.sum += value
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
			OboeLog(ERROR, fmt.Sprintf("Failed to record histogram: %v", err))
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

// sets the transactionNameOverflow flag
func setTransactionNameOverflow(flag bool) {
	metricsHTTPMeasurements.lock.Lock()
	metricsHTTPMeasurements.transactionNameOverflow = flag
	metricsHTTPMeasurements.lock.Unlock()
}

// adds a measurement to a BSON buffer
// bbuf		the BSON buffer to append the metric to
// index	a running integer (0,1,2,...) which is needed for BSON arrays
// m		measurement to be added
func addMeasurementToBSON(bbuf *bsonBuffer, index *int, m *measurement) {
	start := bsonAppendStartObject(bbuf, strconv.Itoa(*index))

	bsonAppendString(bbuf, "name", m.name)
	bsonAppendInt(bbuf, "count", m.count)
	if m.reportSum {
		bsonAppendFloat64(bbuf, "sum", m.sum)
	}

	if len(m.tags) > 0 {
		start := bsonAppendStartObject(bbuf, "tags")
		for k, v := range m.tags {
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
		OboeLog(ERROR, fmt.Sprintf("Failed to encode histogram: %v", err))
		return
	}

	start := bsonAppendStartObject(bbuf, strconv.Itoa(*index))

	bsonAppendString(bbuf, "name", "TransactionResponseTime")
	bsonAppendBinary(bbuf, "value", data)

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
}

func incrementNumSent(count int) {
	metricsEventQueueStats.lock.Lock()
	metricsEventQueueStats.numSent += int64(count)
	metricsEventQueueStats.lock.Unlock()
}

func incrementNumOverflowed(count int) {
	metricsEventQueueStats.lock.Lock()
	metricsEventQueueStats.numOverflowed += int64(count)
	metricsEventQueueStats.lock.Unlock()
}

func incrementNumFailed(count int) {
	metricsEventQueueStats.lock.Lock()
	metricsEventQueueStats.numFailed += int64(count)
	metricsEventQueueStats.lock.Unlock()
}

func incrementTotalEvents(count int) {
	metricsEventQueueStats.lock.Lock()
	metricsEventQueueStats.totalEvents += int64(count)
	metricsEventQueueStats.lock.Unlock()
}

func setQueueLargest(count int) {
	metricsEventQueueStats.lock.Lock()
	if int64(count) > metricsEventQueueStats.queueLargest {
		metricsEventQueueStats.queueLargest = int64(count)
	}
	metricsEventQueueStats.lock.Unlock()
}
