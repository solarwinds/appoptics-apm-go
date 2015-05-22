package traceview

import (
	"unsafe"
)

/*
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"

type Event struct {
	event C.oboe_event_t
}

// Every event needs a label:
type Label string

const (
	LabelEntry = "entry"
	LabelExit  = "exit"
	LabelInfo  = "info"
)

func NewEvent(md *C.oboe_metadata_t, label Label, layer string) *Event {
	var event C.oboe_event_t
	C.oboe_event_init(&event, md)
	e := &Event{event}
	e.addLabelLayer(label, layer)
	return e
}

func (e *Event) addLabelLayer(label Label, layer string) {
	e.AddString("Label", string(label))
	if layer != "" {
		e.AddString("Layer", layer)
	}
}

// Adds string key/value to event
func (e *Event) AddString(key, value string) {
	var ckey *C.char = C.CString(key)
	var cvalue *C.char = C.CString(value)

	C.oboe_event_add_info(&e.event, ckey, cvalue)

	C.free(unsafe.Pointer(ckey))
	C.free(unsafe.Pointer(cvalue))
}

// Adds int key/value to event
func (e *Event) AddInt(key string, value int) {
	var ckey *C.char = C.CString(key)
	C.oboe_event_add_info_int64(&e.event, ckey, C.int64_t(value))
	C.free(unsafe.Pointer(ckey))
}

// Adds int64 key/value to event
func (e *Event) AddInt64(key string, value int64) {
	var ckey *C.char = C.CString(key)
	C.oboe_event_add_info_int64(&e.event, ckey, C.int64_t(value))
	C.free(unsafe.Pointer(ckey))
}

// Adds int32 key/value to event
func (e *Event) AddInt32(key string, value int32) {
	var ckey *C.char = C.CString(key)
	C.oboe_event_add_info_int64(&e.event, ckey, C.int64_t(value))
	C.free(unsafe.Pointer(ckey))
}

// Adds float32 key/value to event
func (e *Event) AddFloat32(key string, value float32) {
	var ckey *C.char = C.CString(key)
	C.oboe_event_add_info_double(&e.event, ckey, C.double(value))
	C.free(unsafe.Pointer(ckey))
}

// Adds float64 key/value to event
func (e *Event) AddFloat64(key string, value float64) {
	var ckey *C.char = C.CString(key)
	C.oboe_event_add_info_double(&e.event, ckey, C.double(value))
	C.free(unsafe.Pointer(ckey))
}

// Adds float key/value to event
func (e *Event) AddBool(key string, value bool) {
	var ckey *C.char = C.CString(key)
	true := 0
	if value {
		true = 1
	}
	C.oboe_event_add_info_bool(&e.event, ckey, C.int(true))
	C.free(unsafe.Pointer(ckey))
}

// Adds edge (reference to previous event) to event
func (e *Event) AddEdge(ctx *Context) {
	C.oboe_event_add_edge(&e.event, &ctx.metadata)
}

func (e *Event) AddEdgeFromMetaDataString(mdstr string) {
	var cmdstr *C.char = C.CString(mdstr)
	var md C.oboe_metadata_t
	C.oboe_metadata_init(&md)
	C.oboe_metadata_fromstr(&md, cmdstr, C.size_t(len(mdstr)))
	C.oboe_event_add_edge(&e.event, &md)
	C.free(unsafe.Pointer(cmdstr))
}

// Reports event using specified Reporter
func (e *Event) ReportUsing(c *Context, r *Reporter) error {
	return r.ReportEvent(c, e)
}

// Reports event using default (UDP) Reporter
func (e *Event) Report(c *Context) error {
	return e.ReportUsing(c, udp_reporter)
}

// Returns Metadata string (X-Trace header)
func (e *Event) MetaDataString() string {
	return metadataString(&e.event.metadata)
}
