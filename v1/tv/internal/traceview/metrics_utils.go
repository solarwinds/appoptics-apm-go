// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"os"
	"syscall"
	"net"
	"strconv"
	"strings"
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
	id, err = am.getContainerId()
	if err == nil {
		id = "container:" + id
		return id
	}

	id, err = am.getAWSInstanceId()
	if err == nil {
		id = "aws:" + id
		return id
	}
	// getMacList always returns a string.
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
		am.cachedSysMeta[BSON_KEY_HOST_ID] = distro
	}()

	// Note: Order of checking is important because some distros share same file names but with different function.
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
		am.cachedSysMeta[BSON_KEY_HOST_ID] = macs
	}()

	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		//TODO: get rid of loopback interfaces
		if mac := iface.HardwareAddr.String(); mac != "" {
			macs += iface.HardwareAddr.String() + ","
		}
	}
	return macs
}

// appendEC2InstanceID appends the EC2 Instance ID if any
func (am *metricsAggregator) appendEC2InstanceID(bbuf *bsonBuffer) {
	//TODO
}

// appendEC2InstanceZone appends the EC2 zone information to the BSON buffer
func (am *metricsAggregator) appendEC2InstanceZone(bbuf *bsonBuffer) {
	//TODO
}

// appendContainerID appends the docker container ID to the BSON buffer
func (am *metricsAggregator) appendContainerID(bbuf *bsonBuffer) {
	//TODO
}

// appendTimestamp appends the timestamp information to the BSON buffer
func (am *metricsAggregator) appendTimestamp(bbuf *bsonBuffer) {
	//TODO
}

// appendFlushInterval appends the flush interval to the BSON buffer
func (am *metricsAggregator) appendFlushInterval(bbuf *bsonBuffer) {
	//TODO
}

// appendTransactionNameOverflow appends the transaction name overflow flag to BSON buffer
func (am *metricsAggregator) appendTransactionNameOverflow(bbuf *bsonBuffer) {
	// TODO
}

// getContainerId gets the Docker Container ID, if any
func (am *metricsAggregator) getContainerId() (id string, err error) {
	// TODO
}

// getAWSInstanceId gets the AWS instance ID, if any
func (am *metricsAggregator) getAWSInstanceId() (id string, err error) {
	// TODO
}
