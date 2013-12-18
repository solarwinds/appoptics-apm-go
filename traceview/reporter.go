package traceview

import (
	"errors"
	"fmt"
	"log"
	"unsafe"
)

/*
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"

type Reporter struct {
	reporter C.oboe_reporter_t
}

func NewUDPReporter() *Reporter {
	r := Reporter{}
	var host *C.char = C.CString("127.0.0.1")
	var port *C.char = C.CString("7831")

	if C.oboe_reporter_udp_init(&r.reporter, host, port) < 0 {
		log.Printf("Failed to initialize UDP reporter")
	}

	C.free(unsafe.Pointer(host))
	C.free(unsafe.Pointer(port))

	return &r
}

func (r *Reporter) ReportEvent(ctx *Context, e *Event) error {
	var err error
	rc := C.oboe_reporter_send(&r.reporter, &ctx.metadata, &e.event)
	if rc < 0 {
		err = errors.New(fmt.Sprintf("Error Reporting Event: %v", int(rc)))
	}
	return err
}
