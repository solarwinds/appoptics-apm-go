package traceview

import (
	"log"
	"net"
	"os"
	"time"
)

// XXX: make this an interface ...
type Reporter struct {
	conn *net.UDPConn
}

func NewUDPReporter() *Reporter {
	// XXX: make connection configurable?
	var conn *net.UDPConn
	serverAddr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:7831")
	if err == nil {
		conn, err = net.DialUDP("udp4", nil, serverAddr)
	}
	if err != nil {
		log.Printf("Failed to initialize UDP reporter: %v", err)
	}
	return &Reporter{conn}
}

func (r *Reporter) ReportEvent(ctx *Context, e *Event) error {
	if r.conn == nil {
		// Reporter didn't initialize, nothing to do...
		return nil
	}

	// XXX add validation from oboe_reporter_send

	us := time.Now().UnixNano() / 1000
	e.AddInt64("Timestamp_u", us)

	// XXX could cache hostname and PID:
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Unable to get hostname: %v", err)
		return err
	}
	e.AddString("Hostname", hostname)
	e.AddInt("PID", os.Getpid())

	// Update the context's op_id to that of the event
	oboe_ids_set_op_id(&ctx.metadata.ids, e.metadata.ids.op_id)

	// Send BSON:
	bson_buffer_finish(&e.bbuf)
	_, err = r.conn.Write(e.bbuf.buf)
	if err != nil {
		log.Printf("Unable to send event: %v", err)
		return err
	}
	return err
}
