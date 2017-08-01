// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import "os"

const (
	BSON_KEY_HOST_ID = "HostID"
	BSON_KEY_UUID = "UUID"
	BSON_KEY_HOSTNAME = "Hostname"
	BSON_KEY_DISTRO = "Distro"
	BSON_KEY_PID = "PID"
	BSON_KEY_TID = "TID"
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

const (
	REDHAT = "/etc/redhat-release"
	AMAZON = "/etc/release-cpe"
	// TODO
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
func (am *metricsAggregator) getHostId() (idStr string) {
	var id string
	var err error

	if id, ok := am.cachedSysMeta[BSON_KEY_HOST_ID]; ok {
		return id
	}

	// Use cached hostID
	defer func() {
		if idStr != "" {
			am.cachedSysMeta[BSON_KEY_HOST_ID] = idStr
		}
	}()

	// Calculate the ID
	id, err = am.getContainerId()
	if err == nil {
		idStr = "container:" + id
		return idStr
	}

	id, err = am.getAWSInstanceId()
	if err == nil {
		idStr = "aws:" + id
		return idStr
	}

	id, err = am.getMACList()
	if err == nil {
		idStr = "mac:" + id
		return idStr
	}

	idStr = "undefined"
	OboeLog(WARNING, "getHostId(): could not retrieve host ID. Last error", err)

	return idStr
}

// appendDistro appends the distro information to BSON buffer
func (am *metricsAggregator) appendDistro(bbuf *bsonBuffer) {
	bsonAppendString(bbuf, BSON_KEY_DISTRO, getDistro())
	//TODO
}

// getDistro retrieves the distribution information and caches it in metricsAggregator
func (am *metricsAggregator) getDistro(bbuf *bsonBuffer) (distro string) {
	if distro, ok := am.cachedSysMeta[BSON_KEY_DISTRO]; ok {
		return distro
	}

	// Use cached hostID
	defer func() {
		if distro != "" {
			am.cachedSysMeta[BSON_KEY_HOST_ID] = distro
		}
	}()

	return distro
}

// appendPID appends the process ID to the BSON buffer
func (am *metricsAggregator) appendPID(bbuf *bsonBuffer) {
	//TODO
}

// appendTID appends the thread ID to the BSON buffer
func (am *metricsAggregator) appendTID(bbuf *bsonBuffer) {
	//TODO
}

// appendSysName appends the sysname to BSON buffer
func (am *metricsAggregator) appendSysName(bbuf *bsonBuffer) {
	//TODO
}

// appendVersion appends the version info the BSON buffer
func (am *metricsAggregator) appendVersion(bbuf *bsonBuffer) {
	//TODO
}

// appendIPAddresses appends the IP addresses to the BSON buffer
func (am *metricsAggregator) appendIPAddresses(bbuf *bsonBuffer) {
	//TODO
}

// appendMAC appends the MAC addresses to the BSON buffer
func (am *metricsAggregator) appendMAC(bbuf *bsonBuffer) {
	// TODO
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

// getMACList gets the MAC address(es)
func (am *metricsAggregator) getMACList() (id string, err error) {
	// TODO
}
