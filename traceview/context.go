package traceview

import (
	"errors"
	"fmt"
	"unsafe"
)

/*
#cgo pkg-config: openssl
#cgo LDFLAGS: -loboe
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"

type Context struct {
	metadata C.oboe_metadata_t
}

// a Context that may or not be tracing (due to sample rate)
type SampledContext interface {
	ReportEvent(label Label, layer string, args ...interface{}) error
}

// a NullContext never reports events
type NullContext struct{}

func (e *NullContext) ReportEvent(label Label, layer string, args ...interface{}) error {
	return nil
}

// Allocates context with random metadata (new trace)
func NewContext() *Context {
	ctx := &Context{}
	C.oboe_metadata_init(&ctx.metadata)
	C.oboe_metadata_random(&ctx.metadata)
	return ctx
}

// Allocates context with existing metadata (for continuing traces)
func NewContextFromMetaDataString(mdstr string) *Context {
	var cmdstr *C.char = C.CString(mdstr)
	ctx := &Context{}
	C.oboe_metadata_init(&ctx.metadata)
	C.oboe_metadata_fromstr(&ctx.metadata, cmdstr, C.size_t(len(mdstr)))
	C.free(unsafe.Pointer(cmdstr))
	return ctx
}

func NewSampledContext(layer, mdstr string, reportEntry bool) (ctx SampledContext) {
	sampled, rate, source := ShouldTraceRequest(layer, mdstr)
	if sampled {
		ctx = NewContext()
		if reportEntry {
			ctx.(*Context).reportEvent(LabelEntry, layer, false, "SampleRate", rate, "SampleSource", source)
		}
	} else {
		ctx = &NullContext{}
	}
	return
}

func (ctx *Context) NewEvent(label Label, layer string) *Event {
	return NewEvent(&ctx.metadata, label, layer)
}

// Create and report an event using KVs from variadic args
func (ctx *Context) ReportEvent(label Label, layer string, args ...interface{}) error {
	return ctx.reportEvent(label, layer, true, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *Context) reportEvent(label Label, layer string, addCtxEdge bool, args ...interface{}) error {
	// create new event from context
	e := ctx.NewEvent(label, layer)
	for i := 0; i < len(args); i += 2 {
		// load key name
		key, is_str := args[i].(string)
		if !is_str {
			return errors.New(fmt.Sprintf("Key %v (type %T) not a string", key, key))
		}
		// load value and add KV to event
		switch val := args[i+1].(type) {
		case string:
			e.AddString(key, val)
		case int:
			e.AddInt(key, val)
		case int64:
			e.AddInt64(key, val)
		case int32:
			e.AddInt32(key, val)
		case float32:
			e.AddFloat32(key, val)
		case float64:
			e.AddFloat64(key, val)
		case bool:
			e.AddBool(key, val)
		case *Context:
			if key == "Edge" {
				e.AddEdge(val)
			}
		default: // XXX log error?
			// fmt.Fprintf(os.Stderr, "Unrecognized Event key %v val %v", key, val)
		}
	}

	if addCtxEdge {
		e.AddEdge(ctx)
	}

	// report event
	return e.Report(ctx)
}

func (ctx *Context) String() string {
	return fmt.Sprintf("Context: %s", metadataString(&ctx.metadata))
}

// converts metadata (*oboe_metadata_t) to a string representation
func metadataString(metadata *C.oboe_metadata_t) string {
	buf := make([]C.char, 64)
	var md_str string

	// XXX: Review: Call C function using Go-managed memory
	rc := C.oboe_metadata_tostr(metadata, &buf[0], C.size_t(len(buf)))
	if rc == 0 {
		md_str = C.GoString(&buf[0])
	}

	return md_str
}
