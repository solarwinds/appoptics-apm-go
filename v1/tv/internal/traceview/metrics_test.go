// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"bytes"
	"log"
	"math"
	"net"
	"os"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/librato/go-traceview/v1/tv/internal/hdrhist"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func bsonToMap(bbuf *bsonBuffer) map[string]interface{} {
	m := make(map[string]interface{})
	bson.Unmarshal(bbuf.GetBuf(), m)
	return m
}

func round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}

func TestDistro(t *testing.T) {
	distro := strings.ToLower(getDistro())

	assert.NotEmpty(t, distro)

	if runtime.GOOS == "linux" {
		assert.NotContains(t, distro, "unknown")
	} else {
		assert.Contains(t, distro, "unknown")
	}
}

func TestAppendUname(t *testing.T) {
	bbuf := NewBsonBuffer()
	appendUname(bbuf)
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	var sysname, version string

	if runtime.GOOS == "linux" {
		var uname syscall.Utsname
		if err := syscall.Uname(&uname); err == nil {
			sysname = Byte2String(uname.Sysname[:])
			version = Byte2String(uname.Version[:])
			sysname = strings.TrimRight(sysname, "\x00")
			version = strings.TrimRight(version, "\x00")
		}

		assert.Equal(t, sysname, m["UnameSysName"])
		assert.Equal(t, version, m["UnameVersion"])
	} else {
		assert.Nil(t, m["UnameSysName"])
		assert.Nil(t, m["UnameVersion"])
	}
}

func TestAppendIPAddresses(t *testing.T) {
	bbuf := NewBsonBuffer()
	appendIPAddresses(bbuf)
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	assert.NotZero(t, m["IPAddresses"])
	bsonIPs := m["IPAddresses"].([]interface{})
	assert.NotZero(t, len(bsonIPs))

	addrs, _ := net.InterfaceAddrs()
	var addresses []string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			addresses = append(addresses, ipnet.IP.String())
		}
	}

	for _, ip1 := range bsonIPs {
		found := false
		for _, ip2 := range addresses {
			if ip1 == ip2 {
				found = true
				break
			}
		}
		assert.True(t, found)
	}
}

func TestAppendMACAddresses(t *testing.T) {
	bbuf := NewBsonBuffer()
	appendMACAddresses(bbuf)
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	assert.NotZero(t, m["MACAddresses"])
	bsonMACs := m["MACAddresses"].([]interface{})
	assert.NotZero(t, len(bsonMACs))

	ifaces, _ := net.Interfaces()
	var macs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if mac := iface.HardwareAddr.String(); mac != "" {
			macs = append(macs, iface.HardwareAddr.String())
		}
	}

	for _, mac1 := range bsonMACs {
		found := false
		for _, mac2 := range macs {
			if mac1 == mac2 {
				found = true
				break
			}
		}
		assert.True(t, found)
	}
}

func TestGetAWSInstanceID(t *testing.T) {
	id := getAWSInstanceID()
	if isEC2Instance() {
		assert.NotEmpty(t, id)
	} else {
		assert.Empty(t, id)
	}
}

func TestGetAWSInstanceZone(t *testing.T) {
	zone := getAWSInstanceZone()
	if isEC2Instance() {
		assert.NotEmpty(t, zone)
	} else {
		assert.Empty(t, zone)
	}
}

func TestGetContainerId(t *testing.T) {
	id := getContainerId()
	if getLineByKeyword("/proc/self/cgroup", "docker") != "" {
		assert.NotEmpty(t, id)
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]+$`), id)
	} else {
		assert.Empty(t, id)
	}
}

func TestAddMetricsValue(t *testing.T) {
	index := 0
	bbuf := NewBsonBuffer()
	addMetricsValue(bbuf, &index, "name1", int(111))
	addMetricsValue(bbuf, &index, "name2", int64(222))
	addMetricsValue(bbuf, &index, "name3", float32(333.33))
	addMetricsValue(bbuf, &index, "name4", float64(444.44))
	addMetricsValue(bbuf, &index, "name5", "hello")
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	assert.NotZero(t, m["0"])
	m2 := m["0"].(map[string]interface{})
	assert.Equal(t, "name1", m2["name"])
	assert.Equal(t, int(111), m2["value"])

	assert.NotZero(t, m["1"])
	m2 = m["1"].(map[string]interface{})
	assert.Equal(t, "name2", m2["name"])
	assert.Equal(t, int64(222), m2["value"])

	assert.NotZero(t, m["2"])
	m2 = m["2"].(map[string]interface{})
	assert.Equal(t, "name3", m2["name"])
	f64 := m2["value"].(float64)
	assert.Equal(t, float64(333.33), round(f64, .5, 2))

	assert.NotZero(t, m["3"])
	m2 = m["3"].(map[string]interface{})
	assert.Equal(t, "name4", m2["name"])
	assert.Equal(t, float64(444.44), m2["value"])

	assert.NotZero(t, m["4"])
	m2 = m["4"].(map[string]interface{})
	assert.Equal(t, "name5", m2["name"])
	assert.Equal(t, "unknown", m2["value"])
}

func TestGetTransactionFromURL(t *testing.T) {
	type record struct {
		url         string
		transaction string
	}
	var test = []record{
		record{
			"https://github.com/librato/go-traceview/blob/metrics/reporter.go#L867",
			"/librato/go-traceview",
		},
		record{
			"http://github.com/librato",
			"/librato",
		},
		record{
			"http://github.com",
			"/",
		},
		record{
			"github.com/librato/go-traceview/blob",
			"/librato/go-traceview",
		},
		record{
			"github.com:8080/librato/go-traceview/blob",
			"/librato/go-traceview",
		},
		record{
			" ",
			"/",
		},
	}

	for _, r := range test {
		assert.Equal(t, r.transaction, getTransactionFromURL(r.url), "url: "+r.url)
	}
}

func TestIsWithinLimit(t *testing.T) {
	m := make(map[string]bool)
	assert.True(t, isWithinLimit(&m, "t1", 3))
	assert.True(t, isWithinLimit(&m, "t2", 3))
	assert.True(t, isWithinLimit(&m, "t3", 3))
	assert.False(t, isWithinLimit(&m, "t4", 3))
	assert.True(t, isWithinLimit(&m, "t2", 3))
}

func TestRecordMeasurement(t *testing.T) {
	var me = &measurements{
		measurements: make(map[string]*measurement),
	}

	t1 := make(map[string]string)
	t1["t1"] = "tag1"
	t1["t2"] = "tag2"
	recordMeasurement(me, "name1", &t1, 111.11, 1, false)
	recordMeasurement(me, "name1", &t1, 222, 1, false)
	assert.NotNil(t, me.measurements["name1&false&t1:tag1&t2:tag2&"])
	m := me.measurements["name1&false&t1:tag1&t2:tag2&"]
	assert.Equal(t, "tag1", m.tags["t1"])
	assert.Equal(t, "tag2", m.tags["t2"])
	assert.Equal(t, 333.11, m.sum)
	assert.Equal(t, 2, m.count)
	assert.False(t, m.reportSum)

	t2 := make(map[string]string)
	t2["t3"] = "tag3"
	recordMeasurement(me, "name2", &t2, 123.456, 3, true)
	assert.NotNil(t, me.measurements["name2&true&t3:tag3&"])
	m = me.measurements["name2&true&t3:tag3&"]
	assert.Equal(t, "tag3", m.tags["t3"])
	assert.Equal(t, 123.456, m.sum)
	assert.Equal(t, 3, m.count)
	assert.True(t, m.reportSum)
}

func TestRecordHistogram(t *testing.T) {
	var hi = &histograms{
		histograms: make(map[string]*histogram),
	}

	recordHistogram(hi, "", time.Duration(123))
	recordHistogram(hi, "", time.Duration(1554))
	assert.NotNil(t, hi.histograms[""])
	h := hi.histograms[""]
	assert.Empty(t, h.tags["TransactionName"])
	encoded, _ := hdrhist.EncodeCompressed(h.hist)
	assert.Equal(t, "HISTFAAAACR42pJpmSzMwMDAxIAKGEHEtclLGOw/QASYmAABAAD//1nj", string(encoded))

	recordHistogram(hi, "hist1", time.Duration(453122))
	assert.NotNil(t, hi.histograms["hist1"])
	h = hi.histograms["hist1"]
	assert.Equal(t, "hist1", h.tags["TransactionName"])
	encoded, _ = hdrhist.EncodeCompressed(h.hist)
	assert.Equal(t, "HISTFAAAACR42pJpmSzMwMDAxIAKGEHEtclLGOw/QAQEmQABAAD//1oB", string(encoded))

	var buf bytes.Buffer
	log.SetOutput(&buf)
	recordHistogram(hi, "hist2", time.Duration(4531224545454563))
	log.SetOutput(os.Stderr)
	assert.Contains(t, buf.String(), "Failed to record histogram: value to large")
}

func TestAddMeasurementToBSON(t *testing.T) {
	veryLongTagName := "verylongnameAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	veryLongTagValue := "verylongtagAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	veryLongTagNameTrimmed := veryLongTagName[0:64]
	veryLongTagValueTrimmed := veryLongTagValue[0:255]

	tags1 := make(map[string]string)
	tags1["t1"] = "tag1"
	tags2 := make(map[string]string)
	tags2[veryLongTagName] = veryLongTagValue

	measurement1 := &measurement{
		name:      "name1",
		tags:      tags1,
		count:     45,
		sum:       592.42,
		reportSum: false,
	}
	measurement2 := &measurement{
		name:      "name2",
		tags:      tags2,
		count:     777,
		sum:       6530.3,
		reportSum: true,
	}

	index := 0
	bbuf := NewBsonBuffer()
	addMeasurementToBSON(bbuf, &index, measurement1)
	addMeasurementToBSON(bbuf, &index, measurement2)
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	assert.NotZero(t, m["0"])
	m1 := m["0"].(map[string]interface{})
	assert.Equal(t, "name1", m1["name"])
	assert.Equal(t, 45, m1["count"])
	assert.Nil(t, m1["sum"])
	assert.NotZero(t, m1["tags"])
	t1 := m1["tags"].(map[string]interface{})
	assert.Equal(t, "tag1", t1["t1"])

	assert.NotZero(t, m["1"])
	m2 := m["1"].(map[string]interface{})
	assert.Equal(t, "name2", m2["name"])
	assert.Equal(t, 777, m2["count"])
	assert.Equal(t, 6530.3, m2["sum"])
	assert.NotZero(t, m2["tags"])
	t2 := m2["tags"].(map[string]interface{})
	assert.Nil(t, t2[veryLongTagName])
	assert.Equal(t, veryLongTagValueTrimmed, t2[veryLongTagNameTrimmed])
}

func TestAddHistogramToBSON(t *testing.T) {
	veryLongTagName := "verylongnameAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	veryLongTagValue := "verylongtagAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	veryLongTagNameTrimmed := veryLongTagName[0:64]
	veryLongTagValueTrimmed := veryLongTagValue[0:255]

	tags1 := make(map[string]string)
	tags1["t1"] = "tag1"
	tags2 := make(map[string]string)
	tags2[veryLongTagName] = veryLongTagValue

	h1 := &histogram{
		hist: hdrhist.WithConfig(hdrhist.Config{
			LowestDiscernible: 1,
			HighestTrackable:  3600000000,
			SigFigs:           3,
		}),
		tags: tags1,
	}
	h1.hist.Record(34532123)
	h2 := &histogram{
		hist: hdrhist.WithConfig(hdrhist.Config{
			LowestDiscernible: 1,
			HighestTrackable:  3600000000,
			SigFigs:           3,
		}),
		tags: tags2,
	}
	h2.hist.Record(39023)

	index := 0
	bbuf := NewBsonBuffer()
	addHistogramToBSON(bbuf, &index, h1)
	addHistogramToBSON(bbuf, &index, h2)
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	assert.NotZero(t, m["0"])
	m1 := m["0"].(map[string]interface{})
	assert.Equal(t, "TransactionResponseTime", m1["name"])
	assert.Equal(t, "HISTFAAAACh42pJpmSzMwMDAwgABzFCaEURcm7yEwf4DRGBnAxMTIAAA//9n9AXI", string(m1["value"].([]byte)))
	assert.NotZero(t, m1["tags"])
	t1 := m1["tags"].(map[string]interface{})
	assert.Equal(t, "tag1", t1["t1"])

	assert.NotZero(t, m["1"])
	m2 := m["1"].(map[string]interface{})
	assert.Equal(t, "TransactionResponseTime", m2["name"])
	assert.Equal(t, "HISTFAAAACZ42pJpmSzMwMDAzAABMJoRRFybvITB/gNEoDWZCRAAAP//YTIF", string(m2["value"].([]byte)))
	assert.NotZero(t, m2["tags"])
	t2 := m2["tags"].(map[string]interface{})
	assert.Nil(t, t2[veryLongTagName])
	assert.Equal(t, veryLongTagValueTrimmed, t2[veryLongTagNameTrimmed])
}
