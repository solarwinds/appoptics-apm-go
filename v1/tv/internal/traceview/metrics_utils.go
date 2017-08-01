// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"os"
	"syscall"
	"net"
	"strconv"
	"strings"
	"net/http"
	"time"
	"io/ioutil"
)

// Metrics key strings
const (
	BSON_KEY_HOST_ID = "HostID"
	BSON_KEY_UUID = "UUID"
	BSON_KEY_HOSTNAME = "Hostname"
	BSON_KEY_DISTRO = "Distro"
	BSON_KEY_PID = "PID"
	BSON_KEY_SYSNAME = "UnameSysName"
	BSON_KEY_VERSION = "UnameVersion"
	BSON_KEY_IPADDR = "IPAddresses"
	BSON_KEY_MAC = "MACAddresses"
	BSON_KEY_EC2_ID = "EC2InstanceID"
	BSON_KEY_EC2_ZONE = "EC2AvailabilityZone"
	BSON_KEY_DOCKER_CONTAINER_ID = "DockerContainerID"
	BSON_KEY_TIMESTAMP = "Timestamp_u"
	BSON_KEY_FLUSH_INTERVAL = "MetricsFlushInterval"
	BSON_KEY_TRANSACTION_NAME_OVERFLOW = "TransactionNameOverflow"
)

// Linux distributions
const (
	REDHAT = "/etc/redhat-release"
	AMAZON = "/etc/release-cpe"
	UBUNTU = "/etc/lsb-release"
	DEBIAN = "/etc/debian_version"
	SUSE = "/etc/SuSE-release"
	SLACKWARE = "/etc/slackware-version"
	GENTOO = "/etc/gentoo-release"
	OTHER = "/etc/issue"
)

// URL for retrieving AWS instance ID
const (
	URL_FOR_AWS_INSTANCE_ID = "http://169.254.169.254/latest/meta-data/instance-id"
	URL_FOR_AWS_ZONE_ID = "http://169.254.169.254/latest/meta-data/placement/availability-zone"
)

// Timeout for retrieving AWS instance ID
const (
	AWSInstanceIDFetchTimeout = 1 // in seconds
)

const (
	CONTAINER_META_FILE = "/proc/self/cgroup"
)

// TODO: combine and refactor the append* methods: apendGeneric(KEY_NAME, FUNC_TO_GET_THE_STRING)

// appendHostname appends the hostname to the BSON buffer
func (am *metricsAggregator) appendHostname(bbuf *bsonBuffer) {
	hostname, err := os.Hostname()
	if err != nil {
		bsonAppendString(bbuf, BSON_KEY_HOSTNAME, hostname)
	}
}

// appendUUID appends the UUID to the BSON buffer
func (am *metricsAggregator) appendUUID(bbuf *bsonBuffer) {
	bsonAppendString(bbuf, BSON_KEY_UUID, am.getHostId())
}

// getHostId gets the unique host identifier (e.g., AWS instance ID, docker container ID or MAC address)
// This function is not concurrency-safe.
func (am *metricsAggregator) getHostId() (id string) {
	var err error

	if id, ok := am.cachedSysMeta[BSON_KEY_HOST_ID]; ok {
		return id
	}

	// Use cached hostID
	defer func() {
		am.cachedSysMeta[BSON_KEY_HOST_ID] = id
	}()

	// Calculate the ID
	id = am.getContainerID()
	if id != "" {
		id = "container:" + id
		return id
	}

	// getAWSInstanceMeta has a internal cache and always return a string.
	id = am.getAWSInstanceMeta(BSON_KEY_EC2_ID, URL_FOR_AWS_INSTANCE_ID)
	if id != "" {
		id = "aws:" + id
		return id
	}

	// getMacList has a internal cache and always returns a string.
	id = am.getMACList()
	if id != "" {
		id = "mac:" + id
		return id
	}

	// Fallback to undefined, no error is returned.
	id = "undefined"
	OboeLog(WARNING, "getHostId(): could not retrieve host ID. Last error", err)

	return id
}

// appendDistro appends the distro information to BSON buffer
func (am *metricsAggregator) appendDistro(bbuf *bsonBuffer) {
	bsonAppendString(bbuf, BSON_KEY_DISTRO, am.getDistro())
}

// getDistro retrieves the distribution information and caches it in metricsAggregator
func (am *metricsAggregator) getDistro() (distro string) {
	if distro, ok := am.cachedSysMeta[BSON_KEY_DISTRO]; ok {
		return distro
	}

	// Use cached hostID
	defer func() {
		am.cachedSysMeta[BSON_KEY_DISTRO] = distro
	}()

	// Note: Order of checking is important because some distros share same file names
	// but with different function.
	// Keep this order: redhat based -> ubuntu -> debian
	// TODO: get distro/version for various Linux distributions
	return distro
}

// appendPID appends the process ID to the BSON buffer
func (am *metricsAggregator) appendPID(bbuf *bsonBuffer) {
	bsonAppendInt(bbuf, BSON_KEY_PID, os.Getpid())
}

// appendSysName appends the uname.Sysname and Version to BSON buffer
func (am *metricsAggregator) appendUname(bbuf *bsonBuffer) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err == nil {
		bsonAppendString(bbuf, BSON_KEY_SYSNAME, uname.Sysname)
		bsonAppendString(bbuf, BSON_KEY_VERSION, uname.Version)
	}
}

// appendIPAddresses appends the IP addresses to the BSON buffer
// TODO: do we need to cache the IPAddr?
func (am *metricsAggregator) appendIPAddresses(bbuf *bsonBuffer) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}

	start := bsonAppendStartArray(bbuf, BSON_KEY_IPADDR)
	var idx int = 0
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			bsonAppendString(bbuf, strconv.Itoa(idx), ipnet.IP.String())
		}
	}

	bsonAppendFinishObject(bbuf, start)
}

// appendMAC appends the MAC addresses to the BSON buffer
func (am *metricsAggregator) appendMAC(bbuf *bsonBuffer) {
	macs:= am.getMACList()
	if macs == "" {
		return
	}
	// TODO: make sure the start returned is used in FinishObject.
	start := bsonAppendStartArray(bbuf, BSON_KEY_MAC)
	var idx int = 0
	for _, mac := range strings.Split(macs, ",") {
		bsonAppendString(bbuf, strconv.Itoa(idx), mac)
	}
	bsonAppendFinishObject(bbuf, start)
}

func (am *metricsAggregator) getMACList() (macs string) {
	if macs, ok := am.cachedSysMeta[BSON_KEY_MAC]; ok {
		return macs
	}

	// Use cached MACList
	defer func() {
		am.cachedSysMeta[BSON_KEY_MAC] = macs
	}()

	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		if iface.Flags & net.FlagLoopback {
			continue
		}
		if mac := iface.HardwareAddr.String(); mac != "" {
			macs += iface.HardwareAddr.String() + ","
		}
	}
	return macs
}

// appendAWSInstanceID appends the EC2 Instance ID if any
func (am *metricsAggregator) appendAWSInstanceID(bbuf *bsonBuffer) {
	var awsInstanceId = am.getAWSInstanceMeta(BSON_KEY_EC2_ID, URL_FOR_AWS_INSTANCE_ID)
	if awsInstanceId == "" {
		return
	}
	bsonAppendString(bbuf, BSON_KEY_EC2_ID, awsInstanceId)
}

// getAWSInstanceMeta gets the AWS instance metadata, if any
func (am *metricsAggregator) getAWSInstanceMeta(key string, url string) (meta string) {
	if meta, ok := am.cachedSysMeta[key]; ok {
		return meta
	}

	// Use cached Instance metadata
	defer func() {
		am.cachedSysMeta[key] = meta
	}()

	// Retrieve the instance meta from a pre-defined URL
	// It's a synchronous call but we're OK as the metricsSender interval is 1 minute
	// (or 30 seconds?)
	timeout := time.Duration(AWSInstanceIDFetchTimeout * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	// I don't want to retry as the connection is supposed to be reliable if this is
	// an AWS instance.
	resp, err := client.Get(url)
	if err != nil {
		OboeLog(DEBUG, "Timeout in getting AWS metadata, not an AWS instance?", err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	meta = string(body)
	return
}

// appendAWSInstanceZone appends the EC2 zone information to the BSON buffer
func (am *metricsAggregator) appendAWSInstanceZone(bbuf *bsonBuffer) {
	zone := am.getAWSInstanceMeta(BSON_KEY_EC2_ZONE, URL_FOR_AWS_ZONE_ID)
	if zone == "" {
		return
	}
	bsonAppendString(bbuf, BSON_KEY_EC2_ZONE, zone)
}

// appendContainerID appends the docker container ID to the BSON buffer
func (am *metricsAggregator) appendContainerID(bbuf *bsonBuffer) {
	cid := am.getContainerID()
	if cid == "" {
		return
	}
	bsonAppendString(bbuf, BSON_KEY_DOCKER_CONTAINER_ID, cid)
}

// getContainerID retrieves the docker container id, if any, and caches it.
func (am *metricsAggregator) getContainerID() (id string) {
	if id, ok := am.cachedSysMeta[BSON_KEY_DOCKER_CONTAINER_ID]; ok {
		return id
	}

	// Use cached Instance metadata
	defer func() {
		am.cachedSysMeta[BSON_KEY_DOCKER_CONTAINER_ID] = id
	}()

	line := getLineByKeyword(CONTAINER_META_FILE, "docker")
	if line == "" {
		return "" // not found
	}
	tokens := strings.Split(line, "/")
	// A typical line returned by cat /proc/self/cgroup (that's why we expect 3 tokens):
	// 9:devices:/docker/40188af19439697187e3f60b933e7e37c5c41035f4c0b266a51c86c5a0074b25
	if len(tokens) != 3 {
		return ""
	}
	id = tokens[2]
	return
}

// appendTimestamp appends the timestamp information to the BSON buffer
func (am *metricsAggregator) appendTimestamp(bbuf *bsonBuffer) {
	//micro seconds since epoch
	bsonAppendInt64(bbuf, BSON_KEY_TIMESTAMP, int64(time.Now().UnixNano() / 1000))
}

// appendFlushInterval appends the flush interval to the BSON buffer
func (am *metricsAggregator) appendFlushInterval(bbuf *bsonBuffer) {
	bsonAppendInt64(bbuf, BSON_KEY_FLUSH_INTERVAL, int64(agentMetricsInterval/time.Second))
}

// appendTransactionNameOverflow appends the transaction name overflow flag to BSON buffer
func (am *metricsAggregator) appendTransactionNameOverflow(bbuf *bsonBuffer) {
	if len(am.transNames) >= MaxTransactionNames {
		bsonAppendBool(bbuf, BSON_KEY_TRANSACTION_NAME_OVERFLOW, true)
	}
}