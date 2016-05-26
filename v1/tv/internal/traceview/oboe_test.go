// +build traceview

// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"log"
	"net"
	"os"
	"testing"
	"time"

	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

func TestInitMessage(t *testing.T) {
	r := SetTestReporter()

	sendInitMessage()
	assertInitMessage(t, r.Bufs)
}
func assertInitMessage(t *testing.T, bufs [][]byte) {
	g.AssertGraph(t, bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go", "entry"}: {g.OutEdges{}, func(n g.Node) {
			assert.Equal(t, 1, n.Map["__Init"])
			assert.Equal(t, initVersion, n.Map["Go.Oboe.Version"])
			assert.NotEmpty(t, n.Map["Oboe.Version"])
			assert.NotEmpty(t, n.Map["Go.Version"])
		}},
		{"go", "exit"}: {g.OutEdges{{"go", "entry"}}, nil},
	})
}

func TestInitMessageUDP(t *testing.T) {
	reporterAddr = "127.0.0.1:7832"
	globalReporter = newReporter()
	assert.IsType(t, &udpReporter{}, globalReporter)

	addr, err := net.ResolveUDPAddr("udp4", reporterAddr)
	assert.NoError(t, err)
	conn, err := net.ListenUDP("udp4", addr)
	assert.NoError(t, err)
	defer conn.Close()
	var bufs [][]byte
	go func(numBufs int) {
		for ; numBufs > 0; numBufs-- {
			buf := make([]byte, 128*1024)
			n, _, err := conn.ReadFromUDP(buf)
			t.Logf("Got UDP buf len %v err %v", n, err)
			if err != nil {
				log.Printf("UDP listener got err, quitting %v", err)
				break
			}
			bufs = append(bufs, buf[0:n])
		}
	}(2)

	sendInitMessage()
	time.Sleep(50 * time.Millisecond)

	assertInitMessage(t, bufs)
}

func TestOboeTracingMode(t *testing.T) {
	_ = SetTestReporter()
	// make oboeSampleRequest think test reporter is not being used for these tests..
	usingTestReporter = false

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "ALWAYS")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 1) // C.OBOE_TRACE_ALWAYS

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "tHRoUgh")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 2) // C.OBOE_TRACE_THROUGH
	ok, _, _ := oboeSampleRequest("myLayer", "1BXXX")
	assert.True(t, ok)
	ok, _, _ = oboeSampleRequest("myLayer", "")
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "never")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 0) // C.OBOE_TRACE_NEVER
	ok, _, _ = oboeSampleRequest("myLayer", "1BXXX")
	assert.False(t, ok)
	ok, _, _ = oboeSampleRequest("myLayer", "")
	assert.False(t, ok)

	os.Setenv("GO_TRACEVIEW_TRACING_MODE", "")
	readEnvSettings()
	assert.EqualValues(t, globalSettings.settings_cfg.tracing_mode, 1)
}
