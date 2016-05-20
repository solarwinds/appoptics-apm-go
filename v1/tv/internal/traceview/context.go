// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
)

const (
	oboeMetadataStringLen = 58
	maskTaskIDLen         = 0x03
	maskOpIDLen           = 0x08
	maskHasOptions        = 0x04
	maskVersion           = 0xF0

	xtrCurrentVersion      = 1
	oboeMaxTaskIDLen       = 20
	oboeMaxOpIDLen         = 8
	oboeMaxMetadataPackLen = 512
)

// orchestras tune to the oboe
type oboeIDs struct{ taskID, opID []byte }

type oboeMetadata struct {
	ids     oboeIDs
	taskLen int
	opLen   int
}

type context struct {
	metadata oboeMetadata
}

func oboeMetadataInit(md *oboeMetadata) int {
	if md == nil {
		return -1
	}

	md.taskLen = oboeMaxTaskIDLen
	md.opLen = oboeMaxOpIDLen
	md.ids.taskID = make([]byte, oboeMaxTaskIDLen)
	md.ids.opID = make([]byte, oboeMaxTaskIDLen)

	return 0
}

func oboeMetadataRandom(md *oboeMetadata) {
	if md == nil {
		return
	}

	_, err := rand.Read(md.ids.taskID)
	if err != nil {
		return
	}
	_, err = rand.Read(md.ids.opID)
	if err != nil {
		return
	}
}

func oboeRandomOpID(md *oboeMetadata) {
	_, err := rand.Read(md.ids.opID)
	if err != nil {
		return
	}
}

func (ids *oboeIDs) setOpID(opID []byte) {
	copy(ids.opID, opID)
}

/*
 * Pack a metadata struct into a buffer.
 *
 * md       - pointer to the metadata struct
 * task_len - the task_id length to take
 * op_len   - the op_id length to take
 * buf      - the buffer to pack the metadata into
 * buf_len  - the space available in the buffer
 *
 * returns the length of the packed metadata, in terms of uint8_ts.
 */
func oboeMetadataPack(md *oboeMetadata, buf []byte) int {
	if md == nil {
		return -1
	}

	reqLen := md.taskLen + md.opLen + 1

	if len(buf) < reqLen {
		return -1
	}

	var taskBits byte

	/*
	 * Flag field layout:
	 *     7    6     5     4     3     2     1     0
	 * +-----+-----+-----+-----+-----+-----+-----+-----+
	 * |                       |     |     |           |
	 * |        version        | oid | opt |    tid    |
	 * |                       |     |     |           |
	 * +-----+-----+-----+-----+-----+-----+-----+-----+
	 *
	 * tid - task id length
	 *          0 <~> 4, 1 <~> 8, 2 <~> 12, 3 <~> 20
	 * oid - op id length
	 *          (oid + 1) * 4
	 * opt - are options present
	 *
	 * version - the version of X-Trace
	 */
	taskBits = (uint8(md.taskLen) >> 2) - 1

	buf[0] = xtrCurrentVersion << 4
	if taskBits == 4 {
		buf[0] |= 3
	} else {
		buf[0] |= taskBits
	}
	buf[0] |= ((uint8(md.opLen) >> 2) - 1) << 3

	copy(buf[1:1+md.taskLen], md.ids.taskID)
	copy(buf[1+md.taskLen:1+md.taskLen+md.opLen], md.ids.opID)

	return reqLen
}

func oboeMetadataUnpack(md *oboeMetadata, data []byte) int {
	if md == nil {
		return -1
	}

	if len(data) == 0 { // no header to read
		return -1
	}

	flag := data[0]
	var taskLen, opLen int

	/* don't recognize this? */
	if (flag&maskVersion)>>4 != xtrCurrentVersion {
		return -2
	}

	taskLen = (int(flag&maskTaskIDLen) + 1) << 2
	if taskLen == 16 {
		taskLen = 20
	}
	opLen = ((int(flag&maskOpIDLen) >> 3) + 1) << 2

	/* do header lengths describe reality? */
	if (taskLen + opLen + 1) > len(data) { // header contains more bytes than buffer
		return -1
	}

	md.taskLen = taskLen
	md.opLen = opLen

	md.ids.taskID = data[1 : 1+taskLen]
	md.ids.opID = data[1+taskLen : 1+taskLen+opLen]

	return 0
}

func oboeMetadataFromString(md *oboeMetadata, buf string) int {
	if md == nil {
		return -1
	}

	ubuf := make([]byte, oboeMaxMetadataPackLen)

	// a hex string's length would be an even number
	if len(buf)%2 == 1 {
		return -1
	}

	// check if there are more hex bytes than we want
	if len(buf)/2 > oboeMaxMetadataPackLen {
		return -1
	}

	// invalid hex?
	ret, err := hex.Decode(ubuf, []byte(buf))
	if ret != len(buf)/2 || err != nil {
		return -1
	}
	ubuf = ubuf[:ret] // truncate buffer to fit decoded bytes
	return oboeMetadataUnpack(md, ubuf)
}

func oboeMetadataToString(md *oboeMetadata) (string, error) {
	buf := make([]byte, 64)
	result := oboeMetadataPack(md, buf)
	if result < 0 {
		return "", errors.New("unable to pack metadata")
	}
	// encode as hex
	enc := make([]byte, 2*result)
	len := hex.Encode(enc, buf[:result])
	return strings.ToUpper(string(enc[:len])), nil
}

func (md *oboeMetadata) opString() string {
	enc := make([]byte, 2*md.opLen)
	len := hex.Encode(enc, md.ids.opID[:md.opLen])
	return strings.ToUpper(string(enc[:len]))
}

// A SampledContext is an oboe context that may or not be tracing.
type SampledContext interface {
	ReportEvent(label Label, layer string, args ...interface{}) error
	ReportEventMap(label Label, layer string, keys map[string]interface{}) error
	Copy() SampledContext
	IsTracing() bool
	String() string
	NewSampledEvent(label Label, layer string, addCtxEdge bool) SampledEvent
}

// A SampledEvent is an event that may or may not be tracing, created by a SampledContext.
type SampledEvent interface {
	ReportContext(c SampledContext, addCtxEdge bool, args ...interface{}) error
	MetadataString() string
}

// A nullContext is not tracing.
type nullContext struct{}
type nullEvent struct{}

func (e *nullContext) ReportEvent(label Label, layer string, args ...interface{}) error {
	return nil
}
func (e *nullContext) ReportEventMap(label Label, layer string, keys map[string]interface{}) error {
	return nil
}
func (e *nullContext) Copy() SampledContext                                         { return &nullContext{} }
func (e *nullContext) IsTracing() bool                                              { return false }
func (e *nullContext) String() string                                               { return "" }
func (e *nullContext) NewSampledEvent(l Label, y string, g bool) SampledEvent       { return &nullEvent{} }
func (e *nullEvent) ReportContext(c SampledContext, g bool, a ...interface{}) error { return nil }
func (e *nullEvent) MetadataString() string                                         { return "" }

// NewNullContext returns a context that is not tracing.
func NewNullContext() SampledContext { return &nullContext{} }

// newContext allocates a context with random metadata (for a new trace).
func newContext() *context {
	ctx := &context{}
	oboeMetadataInit(&ctx.metadata)
	oboeMetadataRandom(&ctx.metadata)
	return ctx
}

func newContextFromMetadataString(mdstr string) *context {
	ctx := &context{}
	oboeMetadataInit(&ctx.metadata)
	oboeMetadataFromString(&ctx.metadata, mdstr)
	return ctx
}

// NewContext starts a trace, possibly continuing it if mdStr is provided, and reporting an entry event using KVs from cb
func NewContext(layer, mdStr string, reportEntry bool, cb func() map[string]interface{}) (ctx SampledContext) {
	sampled, rate, source := shouldTraceRequest(layer, mdStr)
	if sampled {
		if mdStr == "" {
			ctx = newContext()
		} else {
			ctx = newContextFromMetadataString(mdStr)
		}
		if reportEntry {
			var kvs map[string]interface{}
			if cb != nil {
				kvs = cb()
			}
			if len(kvs) == 0 {
				kvs = make(map[string]interface{})
			}
			kvs["SampleRate"] = rate
			kvs["SampleSource"] = source
			if err := ctx.(*context).reportEventMap(LabelEntry, layer, false, kvs); err != nil {
				ctx = &nullContext{}
			}
		}
	} else {
		ctx = &nullContext{}
	}
	return
}

func (ctx *context) Copy() SampledContext {
	md := oboeMetadata{}
	oboeMetadataInit(&md)
	copy(md.ids.taskID, ctx.metadata.ids.taskID)
	copy(md.ids.opID, ctx.metadata.ids.opID)
	return &context{metadata: md}
}
func (ctx *context) IsTracing() bool { return true }

func (ctx *context) NewEvent(label Label, layer string) *event {
	return newEvent(&ctx.metadata, label, layer)
}

func (ctx *context) NewSampledEvent(label Label, layer string, addCtxEdge bool) SampledEvent {
	e := newEvent(&ctx.metadata, label, layer)
	if addCtxEdge {
		e.AddEdge(ctx)
	}
	return e
}

// Create and report and event using a map of KVs
func (ctx *context) ReportEventMap(label Label, layer string, keys map[string]interface{}) error {
	return ctx.reportEventMap(label, layer, true, keys)
}

func (ctx *context) reportEventMap(label Label, layer string, addCtxEdge bool, keys map[string]interface{}) error {
	var args []interface{}
	for k, v := range keys {
		args = append(args, k)
		args = append(args, v)
	}
	return ctx.reportEvent(label, layer, addCtxEdge, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *context) ReportEvent(label Label, layer string, args ...interface{}) error {
	return ctx.reportEvent(label, layer, true, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *context) reportEvent(label Label, layer string, addCtxEdge bool, args ...interface{}) error {
	// create new event from context
	e := ctx.NewEvent(label, layer)
	return ctx.report(e, addCtxEdge, args...)
}

// report an event using KVs from variadic args
func (ctx *context) report(e *event, addCtxEdge bool, args ...interface{}) error {
	for i := 0; i < len(args); i += 2 {
		// load key name
		key, isStr := args[i].(string)
		if !isStr {
			return fmt.Errorf("Key %v (type %T) not a string", key, key)
		}
		// load value and add KV to event
		switch val := args[i+1].(type) {
		case string:
			e.AddString(key, val)
		case []byte:
			e.AddBinary(key, val)
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
		case *context:
			if key == "Edge" {
				e.AddEdge(val)
			}
		default:
			// silently skip unsupported value type
			if debugLog {
				log.Printf("Unrecognized Event key %v val %v", key, val)
			}
		}
	}

	if addCtxEdge {
		e.AddEdge(ctx)
	}

	// report event
	return e.Report(ctx)
}

func (ctx *context) String() string { return ctx.metadata.String() }

// String returns a hex string representation
func (md *oboeMetadata) String() string {
	mdStr, _ := oboeMetadataToString(md)
	return mdStr
}
