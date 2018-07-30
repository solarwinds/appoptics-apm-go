// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
)

// EC2 Metadata URLs
const (
	// the url to fetch EC2 metadata
	ec2IDURL   = "http://169.254.169.254/latest/meta-data/instance-hostId"
	ec2ZoneURL = "http://169.254.169.254/latest/meta-data/placement/availability-zone"

	// the interval to update the metadata periodically
	observeInterval = time.Minute

	// the maximum wait time for host metadata retrival
	updateTimeout = time.Second * 10

	// the maximum failures for expensive retrivals (EC2, Container ID, etc)
	maxFailCnt = 20
)

// logging texts
const (
	hostObserverStarted = "Host metadata observer started."
	hostObserverStopped = "Host metadata observer stopped."
)

var (
	// the global counter of failed retrievals of AWS metadata. If it keeps
	// failing consecutively for a certain number maxFailCnt, the program
	// may not be running in an EC2 instance.
	// This variable should always be accessed through atomic operators.
	awsMetaFailedCnt int32

	// the same as above, but works for container ID
	dockerMetaFailedCnt int32
)

// observer checks the update of the host metadata periodically. It runs in a
// standalone goroutine.
func observer() {
	log.Debug(hostObserverStarted)

	tm := time.Duration(updateTimeout)
	// initialize the hostID as soon as possible
	timedUpdateHostID(tm, hostId)

	roundup := time.Now().Truncate(observeInterval).Add(observeInterval)
	// Sleep returns immediately if roundup is before time.Now()
	time.Sleep(roundup.Sub(time.Now()))

	tk := time.NewTicker(observeInterval)
	defer func() { tk.Stop() }()

loop:
	for {
		timedUpdateHostID(tm, hostId)

		select {
		case <-tk.C:

		case <-exit:
			break loop
		}
	}

	log.Warning(hostObserverStopped)
}

// funcName returns the function's name in string
func funcName(fn interface{}) string {
	if fn == nil {
		return ""
	}
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

// getOrFallback runs the function provided, and returns the fallback value if
// the function executed returns an empty string
func getOrFallback(fn func() string, fb string) string {
	if s := fn(); s != "" {
		return s
	}
	return fb
}

// timedUpdateHostID tries to update the lockedID but gives up after a specified
// duration
func timedUpdateHostID(d time.Duration, lh *lockedID) {
	// use buffered channel to avoid block the goroutine
	// when we return early
	done := make(chan struct{}, 1)
	tm := time.NewTimer(d)

	go func(c chan struct{}, l *lockedID) {
		updateHostID(l)
		c <- struct{}{}
	}(done, lh)

	select {
	case <-done:
	case <-tm.C:
	}
}

// TODO: which of the following never change? EC2 ID/Zone? ContainerID?
// TODO: herokuID?
func updateHostID(lh *lockedID) {
	old := lh.copyID()

	// compare and fallback if error happens
	hostname := getOrFallback(getHostname, old.hostname)
	pid := PID()
	ec2Id := getOrFallback(getEC2ID, old.ec2Id)
	ec2Zone := getOrFallback(getEC2Zone, old.ec2Zone)
	dockerId := getOrFallback(getContainerID, old.dockerId)
	herokuId := getOrFallback(getHerokuDynoId, old.herokuId)

	mac := getMACAddressList()
	if len(mac) == 0 {
		mac = old.mac
	}

	setters := []IDSetter{
		withHostname(hostname),
		withPid(pid),
		withEC2Id(ec2Id),
		withEC2Zone(ec2Zone),
		withDockerId(dockerId),
		withMAC(mac),
		withHerokuId(herokuId),
	}

	lh.fullUpdate(setters...)
}

// getHostname is the implementation of getting the hostname
func getHostname() string {
	if host, err := os.Hostname(); err != nil {
		log.Warningf("Failed to get hostname: %s", err)
		return ""
	} else {
		return host
	}
}

func getPid() int {
	return os.Getpid()
}

// getAWSMeta fetches the metadata from a specific AWS URL and cache it into
// a provided variable.
// This function increment the value of awsMetaFailedCnt by one for each
// failure, and reset the counter after a successful retrieval. It will bypass
// the work after the value of awsMetaFailedCnt reaches the limit of maxFailCnt.
func getAWSMeta(url string) (meta string) {
	if atomic.LoadInt32(&awsMetaFailedCnt) >= maxFailCnt {
		return
	}

	defer func() {
		if meta != "" {
			atomic.StoreInt32(&awsMetaFailedCnt, 0)
		} else {
			atomic.AddInt32(&awsMetaFailedCnt, 1)
		}
	}()

	// Fetch it from the specified URL if the cache is uninitialized or no
	// cache at all.
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Debugf("Failed to get AWS metadata from %s", url)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Debugf("Failed to read AWS metadata response: %s", url)
		return
	}

	meta = string(body)
	return
}

// gets the AWS instance ID (or empty string if not an AWS instance)
func getEC2ID() string {
	return getAWSMeta(ec2IDURL)
}

// gets the AWS instance zone (or empty string if not an AWS instance)
func getEC2Zone() string {
	return getAWSMeta(ec2ZoneURL)
}

// getContainerID fetches the container ID by reading '/proc/self/cgroup'
// It will stop retrying after a certain amount of consecutive failures,
// which means it may not be running inside a container
func getContainerID() (id string) {
	if atomic.LoadInt32(&dockerMetaFailedCnt) >= maxFailCnt {
		return
	}

	defer func() {
		if id != "" {
			atomic.StoreInt32(&dockerMetaFailedCnt, 0)
		} else {
			atomic.AddInt32(&dockerMetaFailedCnt, 1)
		}
	}()

	id = getContainerIDFromString(func(keyword string) string {
		return utils.GetLineByKeyword("/proc/self/cgroup", keyword)
	})
	return
}

// getContainerIDFromString initializes the docker container ID (or empty
// string if not a docker/ecs container). It accepts a function parameter
// as the source where it gets container metadata from, which makes it more
// flexible and enables better testability.
// A typical line returned by cat /proc/self/cgroup:
// 9:devices:/docker/40188af19439697187e3f60b933e7e37c5c41035f4c0b266a51c86c5a0074b25
func getContainerIDFromString(getter func(string) string) string {
	keywords := []string{"/docker/", "/ecs/", "/kubepods/"}
	isID := regexp.MustCompile("^[0-9a-f]{64}$").MatchString
	line := ""

	for _, keyword := range keywords {
		if line = getter(keyword); line != "" {
			break
		}
	}

	if line == "" {
		return ""
	}

	tokens := strings.Split(line, "/")
	for _, token := range tokens {
		// a length of 64 indicates a container ID
		// ensure token is hex SHA1
		if isID(token) {
			return token
		}
	}
	return ""
}

// gets a comma-separated list of MAC addresses
func getMACAddressList() []string {
	var macAddrs []string

	if ifaces, err := FilteredIfaces(); err != nil {
		return macAddrs
	} else {
		for _, iface := range ifaces {
			if mac := iface.HardwareAddr.String(); mac != "" {
				macAddrs = append(macAddrs, iface.HardwareAddr.String())
			}
		}
	}

	return macAddrs
}

func getHerokuDynoId() string {
	return "not-supported"
}
