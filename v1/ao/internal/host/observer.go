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
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
)

// EC2 Metadata URLs
const (
	// the url to fetch EC2 metadata
	ec2IDURL   = "http://169.254.169.254/latest/meta-data/instance-id"
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
func timedUpdateHostID(d time.Duration, lh *lockedID) bool {
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
		return true
	case <-tm.C:
		return false
	}
}

func updateHostID(lh *lockedID) {
	old := lh.copyID()

	// compare and fallback if error happens
	hostname := getOrFallback(getHostname, old.hostname)
	pid := PID()
	ec2Id := getOrFallback(getEC2ID, old.ec2Id)
	ec2Zone := getOrFallback(getEC2Zone, old.ec2Zone)
	cid := getOrFallback(getContainerID, old.containerId)
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
		withContainerId(cid),
		withMAC(mac),
		withHerokuId(herokuId),
	}

	lh.fullUpdate(setters...)
}

// getHostname is the implementation of getting the hostname
func getHostname() string {
	h, err := os.Hostname()
	if err == nil {
		hm.Lock()
		hostname = h
		hm.Unlock()
	}
	return h
}

func getPid() int {
	return os.Getpid()
}

// getAWSMeta fetches the metadata from a specific AWS URL and cache it into
// a provided variable.
func getAWSMeta(url string) (meta string) {
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
	ec2IdOnce.Do(func() {
		ec2Id = getAWSMeta(ec2IDURL)
		log.Debugf("Got and cached ec2Id: %s", ec2Id)
	})
	return ec2Id
}

// gets the AWS instance zone (or empty string if not an AWS instance)
func getEC2Zone() string {
	ec2ZoneOnce.Do(func() {
		ec2Zone = getAWSMeta(ec2ZoneURL)
		log.Debugf("Got and cached ec2Zone: %s", ec2Zone)
	})
	return ec2Zone
}

// getContainerID fetches the container ID by reading '/proc/self/cgroup'
func getContainerID() (id string) {
	containerIdOnce.Do(func() {
		containerId = getContainerIDFromString(func(keyword string) string {
			return utils.GetLineByKeyword("/proc/self/cgroup", keyword)
		})
		log.Debugf("Got and cached container id: %s", containerId)
	})

	return containerId
}

// getContainerIDFromString initializes the container ID (or empty
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
