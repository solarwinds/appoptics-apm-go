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
	metadata oboe_metadata_t
	bbuf     bson_buffer
}

// Every event needs a label:
type Label string

const (
	LabelEntry = "entry"
	LabelExit  = "exit"
	LabelInfo  = "info"
)

func oboe_event_init(evt *Event, md *oboe_metadata_t) int {
    assert(evt);
    assert(md);

    var result int
    md_buf := make([]byte, 64)

    // Metadata initialization

    result = oboe_metadata_init(&evt.metadata)
    if (result < 0) {
		return result
	}

    evt.metadata.task_len = md.task_len;
    evt.metadata.op_len = md.op_len;

	copy(evt.metadata.ids.task_id, md.ids.task_id)
    oboe_random_op_id(evt.metadata.ids.op_id);

    // Buffer initialization 

    if (!bson_buffer_init(&evt->bbuf)) goto destroy_metadata;

    // Copy header to buffer
    // TODO errors?
    if (!bson_append_string(&evt->bbuf, "_V", header)) goto destroy_buf;

    // Pack metadata
    if (oboe_metadata_tostr(&evt->metadata, md_buf, 64) < 0) goto destroy_buf;

    if(!bson_append_string(&evt->bbuf, "X-Trace", md_buf)) goto destroy_buf;

    return 0;

destroy_buf:
    bson_buffer_destroy(&evt->bbuf);
destroy_metadata:
    oboe_metadata_destroy(&evt->metadata);
fail:
    return -1;
}

func NewEvent(md *oboe_metadata_t, label Label, layer string) *Event {
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
