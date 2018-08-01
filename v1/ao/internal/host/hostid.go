// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"reflect"
	"sync"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
)

// logging texts
const (
	partialUpdateNotAllowed = "Partial Update is not allowed: %d."
	hostIdInitDone          = "HostID initialization done."
)

// caches and their sync.Once protectors
var (
	containerId     string
	containerIdOnce sync.Once

	ec2Id     string
	ec2IdOnce sync.Once

	ec2Zone     string
	ec2ZoneOnce sync.Once
)

// lockedID is a ID protected by a mutex. To avoid being modified without
// mutex protected, the caller can only get a copyID of the internal ID.
type lockedID struct {
	sync.RWMutex

	// it will be closed when the ID is initialized for the first time
	c chan struct{}

	// make sure channel c is not close twice
	cClosed *sync.Once

	id *ID
}

// newLockedID returns an uninitialized lockedID, it can be used only after
// the first full update (see fullUpdate())
func newLockedID() *lockedID {
	return &lockedID{
		RWMutex: sync.RWMutex{},
		c:       make(chan struct{}),
		cClosed: &sync.Once{},
		id:      newID(),
	}
}

func (h *ID) copy() ID {
	c := newID()
	c.update(
		withHostname(h.hostname),
		withPid(h.pid), // pid doesn't change, but we fullUpdate it anyways
		withEC2Id(h.ec2Id),
		withEC2Zone(h.ec2Zone),
		withContainerId(h.containerId),
		withMAC(h.mac),
		withHerokuId(h.herokuId))
	return *c
}

// fullUpdate update all the fields of HostID with the setters. Unlike HostID's
// update function, partial update is not allowed here.
func (lh *lockedID) fullUpdate(setters ...IDSetter) {
	lh.Lock()
	defer lh.Unlock()

	if len(setters) != reflect.ValueOf(lh.id).Elem().NumField() {
		log.Debugf(partialUpdateNotAllowed, len(setters))
		return
	}

	lh.id.update(setters...)
	lh.setReady()
}

// setReady changes the status of the lockID to 'ready'.
func (lh *lockedID) setReady() {
	select {
	case <-lh.c:
		return
	default:
		lh.cClosed.Do(func() {
			close(lh.c)
			log.Info(hostIdInitDone)
		})
	}
}

// ready returns if the lockedID is initialized.
func (lh *lockedID) ready() bool {
	select {
	case <-lh.c:
		return true
	default:
		return false
	}
}

// waitForReady blocks until the lockedID is ready (initialized)
func (lh *lockedID) waitForReady() {
	<-lh.c
}

// copyID returns a copy of the lockedID's internal HostID. However, it doesn't
// check if the internal HostID has been initialized.
func (lh *lockedID) copyID() ID {
	lh.RLock()
	defer lh.RUnlock()

	return lh.id.copy()
}

// ID defines the minimum set of host metadata that identifies a host
type ID struct {
	// the hostname of this host
	hostname string

	// process ID
	pid int

	// EC2 instance ID
	ec2Id string

	// EC2 availability zone
	ec2Zone string

	// container ID
	containerId string

	// the list of MAC addresses
	mac []string

	// The Heroku DynoID
	herokuId string
}

// Hostname returns the hostname field of ID
func (h ID) Hostname() string {
	return h.hostname
}

// Pid returns the pid field of ID
func (h ID) Pid() int {
	return h.pid
}

// EC2Id returns ec2id field of ID
func (h ID) EC2Id() string {
	return h.ec2Id
}

// EC2Zone returns the ec2ZoneURL field of ID
func (h ID) EC2Zone() string {
	return h.ec2Zone
}

// ContainerId returns the containerId field of ID
func (h ID) ContainerId() string {
	return h.containerId
}

// MAC returns the mac field of ID
func (h ID) MAC() []string {
	return h.mac
}

// HerokuID returns the herokuId field of ID
func (h ID) HerokuID() string {
	return h.herokuId
}

// IDSetter defines a function type which set a field of ID
type IDSetter func(h *ID)

func withHostname(hostname string) IDSetter {
	return func(h *ID) {
		h.hostname = hostname
	}
}

func withPid(pid int) IDSetter {
	return func(h *ID) {
		h.pid = pid
	}
}

func withEC2Id(id string) IDSetter {
	return func(h *ID) {
		h.ec2Id = id
	}
}

func withEC2Zone(zone string) IDSetter {
	return func(h *ID) {
		h.ec2Zone = zone
	}
}

func withContainerId(id string) IDSetter {
	return func(h *ID) {
		h.containerId = id
	}
}

func withMAC(mac []string) IDSetter {
	return func(h *ID) {
		h.mac = []string{}
		for _, m := range mac {
			h.mac = append(h.mac, m)
		}
	}
}

func withHerokuId(id string) IDSetter {
	return func(h *ID) {
		h.herokuId = id
	}
}

func newID(setters ...IDSetter) *ID {
	h := &ID{}
	h.update(setters...)
	return h
}

func (h *ID) update(setters ...IDSetter) {
	for _, fn := range setters {
		fn(h)
	}
}
