package traceview

import (
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

func (ctx *Context) NewEvent(label Label, layer string) *Event {
	return NewEvent(&ctx.metadata, label, layer)
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
