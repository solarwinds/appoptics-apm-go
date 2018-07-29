// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLockedHostID(t *testing.T) {
	hostname := "example.com"
	p := 12345
	ec2Id := "ewr234ldsfl342-1"
	ec2Zone := "us-east-7"
	dockerId := "23423jlksl4j2l"
	mac := []string{"72:00:07:e5:23:51", "c6:61:8b:53:d6:b5", "72:00:07:e5:23:50"}
	herokuId := "heroku-test"

	lh := newLockedID()
	assert.False(t, lh.ready())
	// try partial update
	lh.fullUpdate(withHostname(hostname))
	assert.Equal(t, "", lh.copyID().Hostname())

	lh.fullUpdate(
		withHostname(hostname),
		withPid(p), // pid doesn't change, but we fullUpdate it anyways
		withEC2Id(ec2Id),
		withEC2Zone(ec2Zone),
		withDockerId(dockerId),
		withMAC(mac),
		withHerokuId(herokuId))

	assert.True(t, lh.ready())
	lh.setReady()

	lh.waitForReady()
	h := lh.copyID()
	assert.Equal(t, hostname, h.Hostname())
	assert.Equal(t, p, h.Pid())
	assert.Equal(t, ec2Id, h.EC2Id())
	assert.Equal(t, ec2Zone, h.EC2Zone())
	assert.Equal(t, dockerId, h.DockerId())
	assert.Equal(t, mac, h.MAC())
	assert.EqualValues(t, herokuId, h.HerokuID())
}
