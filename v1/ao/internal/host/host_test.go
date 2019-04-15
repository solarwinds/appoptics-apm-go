// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	aolog "github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
)

func init() {
	Start()
}

func TestFilteredIfaces(t *testing.T) {
	ifaces, err := FilteredIfaces()
	if err != nil {
		log.Printf("Got err from FilteredIfaces: %s\n", err)
	}
	if len(ifaces) == 0 {
		log.Println("Got none interfaces.")
	}
	for _, iface := range ifaces {
		assert.Equal(t, net.Flags(0), iface.Flags&net.FlagLoopback)
		assert.Equal(t, net.Flags(0), iface.Flags&net.FlagPointToPoint)
		assert.True(t, IsPhysicalInterface(iface.Name))
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			assert.NotNil(t, net.ParseIP(addr.(*net.IPNet).IP.String()),
				addr.String())
			// actually the reserved loopback addresses are 127.0.0.0/8
			// for IPv4 and ::1/128 for IPv6, but mostly 127.0.0.1 and ::1
			// are used.
			assert.NotEqual(t, "127.0.0.1",
				addr.(*net.IPNet).IP.String(),
				addr.(*net.IPNet).IP.String())
			assert.NotEqual(t, "::1",
				addr.(*net.IPNet).IP.String(),
				addr.(*net.IPNet).IP.String())
		}
	}
}

func TestIPAddresses(t *testing.T) {
	ips := IPAddresses()
	for _, ip := range ips {
		assert.NotNil(t, net.ParseIP(ip))
	}
}

func TestHostname(t *testing.T) {
	assert.NotEmpty(t, Hostname())
}

func TestConfiguredHostname(t *testing.T) {
	var buf utils.SafeBuffer
	var writers []io.Writer

	writers = append(writers, &buf)

	log.SetOutput(io.MultiWriter(writers...))

	defer func() {
		log.SetOutput(os.Stderr)
	}()

	var old string
	var has bool
	old, has = os.LookupEnv("APPOPTICS_HOSTNAME_ALIAS")
	os.Setenv("APPOPTICS_HOSTNAME_ALIAS", "testAlias")
	os.Setenv("APPOPTICS_SERVICE_KEY", "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go")
	config.Load()
	assert.Equal(t, "testAlias", ConfiguredHostname())
	assert.True(t, strings.Contains(buf.String(), "Accepted config items"), buf.String())
	assert.True(t, strings.Contains(buf.String(), "APPOPTICS_HOSTNAME_ALIAS"), buf.String())

	if has {
		os.Setenv("APPOPTICS_HOSTNAME_ALIAS", old)
	}
}

func TestPID(t *testing.T) {
	assert.Equal(t, os.Getpid(), PID())
}

func TestStopHostIDObserver(t *testing.T) {
	var buf utils.SafeBuffer
	var writers []io.Writer

	writers = append(writers, &buf)
	writers = append(writers, os.Stderr)

	log.SetOutput(io.MultiWriter(writers...))

	defer func() {
		log.SetOutput(os.Stderr)
	}()

	aolog.SetLevel(aolog.INFO)
	Stop()
	assert.True(t, strings.Contains(buf.String(),
		stopHostIdObserverByUser), buf.String())
	buf.Reset()
	Stop()
	assert.Equal(t, "", buf.String())
	aolog.SetLevel(aolog.WARNING)
}

func TestCurrentID(t *testing.T) {
	assert.Equal(t, os.Getpid(), CurrentID().Pid())
}

func TestDistro(t *testing.T) {
	distro := strings.ToLower(initDistro())

	assert.NotEmpty(t, distro)

	if runtime.GOOS == "linux" {
		assert.NotContains(t, distro, "unknown")
	} else {
		assert.Contains(t, distro, "unknown")
	}
}
