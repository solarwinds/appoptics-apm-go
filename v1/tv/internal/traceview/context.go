// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

// Low-level API for creating and reporting events for distributed tracing with TraceView
package traceview

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	oboe_metadata_string_len = 58
	mask_task_id_len         = 0x03
	mask_op_id_len           = 0x08
	mask_has_options         = 0x04
	mask_version             = 0xF0

	xtr_current_version        = 1
	oboe_max_task_id_len       = 20
	oboe_max_op_id_len         = 8
	oboe_max_metadata_pack_len = 512
)

type oboe_ids_t struct {
	task_id []byte
	op_id   []byte
}

type oboe_metadata_t struct {
	ids      oboe_ids_t
	task_len int
	op_len   int
}

type Context struct {
	metadata oboe_metadata_t
}

func oboe_metadata_init(md *oboe_metadata_t) int {
	if md == nil {
		return -1
	}

	md.task_len = oboe_max_task_id_len
	md.op_len = oboe_max_op_id_len
	md.ids.task_id = make([]byte, oboe_max_task_id_len)
	md.ids.op_id = make([]byte, oboe_max_task_id_len)

	return 0
}

func oboe_metadata_random(md *oboe_metadata_t) {
	if md == nil {
		return
	}

	_, err := rand.Read(md.ids.task_id)
	if err != nil {
		return
	}
	_, err = rand.Read(md.ids.op_id)
	if err != nil {
		return
	}
}

func oboe_random_op_id(md *oboe_metadata_t) {
	_, err := rand.Read(md.ids.op_id)
	if err != nil {
		return
	}
}

func oboe_ids_set_op_id(ids *oboe_ids_t, op_id []byte) {
	copy(ids.op_id, op_id)
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
func oboe_metadata_pack(md *oboe_metadata_t, buf []byte) int {
	if md == nil {
		return -1
	}

	req_len := md.task_len + md.op_len + 1

	if len(buf) < req_len {
		return -1
	}

	var task_bits byte

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
	task_bits = (uint8(md.task_len) >> 2) - 1

	buf[0] = xtr_current_version << 4
	if task_bits == 4 {
		buf[0] |= 3
	} else {
		buf[0] |= task_bits
	}
	buf[0] |= ((uint8(md.op_len) >> 2) - 1) << 3

	copy(buf[1:1+md.task_len], md.ids.task_id)
	copy(buf[1+md.task_len:1+md.task_len+md.op_len], md.ids.op_id)

	return req_len
}

func oboe_metadata_unpack(md *oboe_metadata_t, data []byte) int {
	if md == nil {
		return -1
	}

	if len(data) == 0 { // no header to read
		return -1
	}

	flag := uint8(data[0])
	var task_len, op_len int

	/* don't recognize this? */
	if (flag&mask_version)>>4 != xtr_current_version {
		return -2
	}

	task_len = (int(flag&mask_task_id_len) + 1) << 2
	if task_len == 16 {
		task_len = 20
	}
	op_len = ((int(flag&mask_op_id_len) >> 3) + 1) << 2

	/* do header lengths describe reality? */
	if (task_len + op_len + 1) > len(data) { // header contains more bytes than buffer
		return -1
	}

	md.task_len = task_len
	md.op_len = op_len

	md.ids.task_id = data[1 : 1+task_len]
	md.ids.op_id = data[1+task_len : 1+task_len+op_len]

	return 0
}

func oboe_metadata_fromstr(md *oboe_metadata_t, buf string) int {
	if md == nil {
		return -1
	}

	ubuf := make([]byte, oboe_max_metadata_pack_len)

	// a hex string's length would be an even number
	if len(buf)%2 == 1 {
		return -1
	}

	// check if there are more hex bytes than we want
	if len(buf)/2 > oboe_max_metadata_pack_len {
		return -1
	}

	// invalid hex?
	ret, err := hex.Decode(ubuf, []byte(buf))
	if ret != len(buf)/2 || err != nil {
		return -1
	}
	ubuf = ubuf[:ret] // truncate buffer to fit decoded bytes
	return oboe_metadata_unpack(md, ubuf)
}

func oboe_metadata_tostr(md *oboe_metadata_t) (string, error) {
	buf := make([]byte, 64)
	result := oboe_metadata_pack(md, buf)
	if result < 0 {
		return "", errors.New("unable to pack metadata")
	}
	// encode as hex
	enc := make([]byte, 2*result)
	len := hex.Encode(enc, buf[:result])
	return strings.ToUpper(string(enc[:len])), nil
}

func (md *oboe_metadata_t) op_string() string {
	enc := make([]byte, 2*md.op_len)
	len := hex.Encode(enc, md.ids.op_id[:md.op_len])
	return strings.ToUpper(string(enc[:len]))
}

// a Context that may or not be tracing (due to sampling)
type SampledContext interface {
	ReportEvent(label Label, layer string, args ...interface{}) error
	ReportEventMap(label Label, layer string, keys map[string]interface{}) error
	Copy() SampledContext
	IsTracing() bool
	String() string
	NewSampledEvent(label Label, layer string, addCtxEdge bool) SampledEvent
}

type SampledEvent interface {
	ReportContext(c SampledContext, addCtxEdge bool, args ...interface{})
	MetadataString() string
}

// a NullContext never reports events
type NullContext struct{}
type NullEvent struct{}

func (e *NullContext) ReportEvent(label Label, layer string, args ...interface{}) error {
	return nil
}
func (e *NullContext) ReportEventMap(label Label, layer string, keys map[string]interface{}) error {
	return nil
}
func (e *NullContext) Copy() SampledContext { return &NullContext{} }
func (e *NullContext) IsTracing() bool      { return false }
func (e *NullContext) String() string       { return "" }
func (e *NullContext) NewSampledEvent(label Label, layer string, addCtxEdge bool) SampledEvent {
	return &NullEvent{}
}

func (e *NullEvent) ReportContext(c SampledContext, addCtxEdge bool, args ...interface{}) {}
func (e *NullEvent) MetadataString() string                                               { return "" }

// Allocates context with random metadata (for a new trace)
func NewContext() *Context {
	ctx := &Context{}
	oboe_metadata_init(&ctx.metadata)
	oboe_metadata_random(&ctx.metadata)
	return ctx
}

// Allocates context with existing metadata (for continuing traces)
func NewContextFromMetadataString(mdstr string) *Context {
	ctx := &Context{}
	oboe_metadata_init(&ctx.metadata)
	oboe_metadata_fromstr(&ctx.metadata, mdstr)
	return ctx
}

func NewSampledContext(layer, mdstr string, reportEntry bool, cb func() map[string]interface{}) (ctx SampledContext) {
	sampled, rate, source := shouldTraceRequest(layer, mdstr)
	if sampled {
		if mdstr == "" {
			ctx = NewContext()
		} else {
			ctx = NewContextFromMetadataString(mdstr)
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
			ctx.(*Context).reportEventMap(LabelEntry, layer, false, kvs)
		}
	} else {
		ctx = &NullContext{}
	}
	return
}

func (ctx *Context) Copy() SampledContext {
	md := oboe_metadata_t{}
	oboe_metadata_init(&md)
	copy(md.ids.task_id, ctx.metadata.ids.task_id)
	copy(md.ids.op_id, ctx.metadata.ids.op_id)
	return &Context{metadata: md}
}
func (ctx *Context) IsTracing() bool { return true }

func (ctx *Context) NewEvent(label Label, layer string) *Event {
	return NewEvent(&ctx.metadata, label, layer)
}

func (ctx *Context) NewSampledEvent(label Label, layer string, addCtxEdge bool) SampledEvent {
	e := NewEvent(&ctx.metadata, label, layer)
	if addCtxEdge {
		e.AddEdge(ctx)
	}
	return e
}

// Create and report and event using a map of KVs
func (ctx *Context) ReportEventMap(label Label, layer string, keys map[string]interface{}) error {
	return ctx.reportEventMap(label, layer, true, keys)
}

func (ctx *Context) reportEventMap(label Label, layer string, addCtxEdge bool, keys map[string]interface{}) error {
	args := make([]interface{}, 0)
	for k, v := range keys {
		args = append(args, k)
		args = append(args, v)
	}
	return ctx.reportEvent(label, layer, addCtxEdge, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *Context) ReportEvent(label Label, layer string, args ...interface{}) error {
	return ctx.reportEvent(label, layer, true, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *Context) reportEvent(label Label, layer string, addCtxEdge bool, args ...interface{}) error {
	// create new event from context
	e := ctx.NewEvent(label, layer)
	return ctx.report(e, addCtxEdge, args...)
}

// report an event using KVs from variadic args
func (ctx *Context) report(e *Event, addCtxEdge bool, args ...interface{}) error {
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
		case *Context:
			if key == "Edge" {
				e.AddEdge(val)
			}
		default:
			// silently skip unsupported value type
			// TODO log error message? return err?
			//fmt.Fprintf(os.Stderr, "Unrecognized Event key %v val %v\n", key, val)
		}
	}

	if addCtxEdge {
		e.AddEdge(ctx)
	}

	// report event
	return e.Report(ctx)
}

func (ctx *Context) String() string {
	return MetadataString(&ctx.metadata)
}

// Converts metadata (*oboe_metadata_t) to hex string representation
func MetadataString(metadata *oboe_metadata_t) string {
	md_str, _ := oboe_metadata_tostr(metadata)
	return md_str
}
