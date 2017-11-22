package traceview

import (
	"fmt"
	"net"
	"os"
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
	udpAddress := os.Getenv("APPOPTICS_REPORTER_UDP")
	if udpAddress == "" {
		udpAddress = udpAddrDefault
	}

	serverAddr, err := net.ResolveUDPAddr("udp4", udpAddress)
	if err == nil {
		conn, err = net.DialUDP("udp4", nil, serverAddr)
	}
	if err != nil {
		OboeLog(ERROR, fmt.Sprintf("TraceView failed to initialize UDP reporter: %v", err))
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

func (r *udpReporter) reportSpan(span *SpanMessage) error { return nil }
