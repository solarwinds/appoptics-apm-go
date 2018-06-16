// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"net"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/agent"
)

const (
	udpAddrDefault = "127.0.0.1:7831"
)

type udpReporter struct {
	conn *net.UDPConn
}

func udpNewReporter() reporter {
	var conn *net.UDPConn
	if reportingDisabled {
		return &nullReporter{}
	}

	// collector address override
	udpAddress := agent.GetConfig(agent.AppOpticsCollectorUDP)
	if udpAddress == "" {
		udpAddress = udpAddrDefault
	}

	serverAddr, err := net.ResolveUDPAddr("udp4", udpAddress)
	if err == nil {
		conn, err = net.DialUDP("udp4", nil, serverAddr)
	}
	if err != nil {
		agent.Error("AppOptics failed to initialize UDP reporter: %v", err)
		return &nullReporter{}
	}

	// add default setting
	updateSetting(int32(TYPE_DEFAULT), "",
		[]byte("SAMPLE_START,SAMPLE_THROUGH_ALWAYS"),
		1000000, 120, argsToMap(16, 8, -1, -1))

	return &udpReporter{conn: conn}
}

func (r *udpReporter) report(ctx *oboeContext, e *event) error {
	if err := prepareEvent(ctx, e); err != nil {
		// don't continue if preparation failed
		return err
	}

	_, err := r.conn.Write((*e).bbuf.GetBuf())
	return err
}

func (r *udpReporter) reportEvent(ctx *oboeContext, e *event) error {
	return r.report(ctx, e)
}

func (r *udpReporter) reportStatus(ctx *oboeContext, e *event) error {
	return r.report(ctx, e)
}

func (r *udpReporter) reportSpan(span SpanMessage) error {
	s := span.(*HTTPSpanMessage)
	bbuf := NewBsonBuffer()
	bsonAppendString(bbuf, "transaction", s.Transaction)
	bsonAppendString(bbuf, "url", s.Path)
	bsonAppendInt(bbuf, "status", s.Status)
	bsonAppendString(bbuf, "method", s.Method)
	bsonAppendBool(bbuf, "hasError", s.HasError)
	bsonAppendInt64(bbuf, "duration", s.Duration.Nanoseconds())
	bsonBufferFinish(bbuf)
	_, err := r.conn.Write(bbuf.buf)
	return err
}
