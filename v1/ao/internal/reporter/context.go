// Copyright (C) 2016 Librato, Inc. All rights reserved.

package reporter

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
)

const (
	oboeMetadataStringLen = 60
	maskTaskIDLen         = 0x03
	maskOpIDLen           = 0x08
	maskHasOptions        = 0x04
	maskVersion           = 0xF0

	xtrCurrentVersion      = 2
	oboeMaxTaskIDLen       = 20
	oboeMaxOpIDLen         = 8
	oboeMaxMetadataPackLen = 512
)

// x-trace flags
const (
	XTR_FLAGS_NONE    = 0x0
	XTR_FLAGS_SAMPLED = 0x1
)

// orchestras tune to the oboe.
type oboeIDs struct{ taskID, opID []byte }

type oboeMetadata struct {
	version uint8
	ids     oboeIDs
	taskLen int
	opLen   int
	flags   uint8
}

type oboeContext struct {
	metadata oboeMetadata
	txCtx    *transactionContext
}

type transactionContext struct {
	name string
	// if the trace/transaction is enabled (defined by per-URL transaction filtering)
	enabled bool
	sync.RWMutex
}

type KVMap map[string]interface{}

// ContextOptions defines the options of creating a context
type ContextOptions struct {
	MdStr string
	// URL is used to do the URL-based transaction filtering.
	URL string

	XTraceOptions bool
	TriggerTrace  bool

	CB func() KVMap
}

// ValidMetadata checks if a metadata string is valid.
func ValidMetadata(mdstr string) bool {
	md := &oboeMetadata{}
	md.Init()
	if err := md.FromString(mdstr); err != nil {
		return false
	}
	return true
}

func (md *oboeMetadata) Init() {
	if md == nil {
		return
	}
	md.version = xtrCurrentVersion
	md.taskLen = oboeMaxTaskIDLen
	md.opLen = oboeMaxOpIDLen
	md.ids.taskID = make([]byte, oboeMaxTaskIDLen)
	md.ids.opID = make([]byte, oboeMaxOpIDLen)
}

// randReader provides random IDs, and can be overridden for testing.
// set by default to read from the crypto/rand Reader.
var randReader = rand.Reader

func (md *oboeMetadata) SetRandom() error {
	if md == nil {
		return errors.New("md.SetRandom: nil md")
	}
	_, err := randReader.Read(md.ids.taskID)
	if err != nil {
		return err
	}
	_, err = randReader.Read(md.ids.opID)
	return err
}

func (md *oboeMetadata) SetRandomOpID() error {
	_, err := randReader.Read(md.ids.opID)
	return err
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
func (md *oboeMetadata) Pack(buf []byte) (int, error) {
	if md == nil {
		return 0, errors.New("md.Pack: nil md")
	}
	if md.taskLen == 0 || md.opLen == 0 {
		return 0, errors.New("md.Pack: invalid md (0 len)")
	}

	reqLen := md.taskLen + md.opLen + 2

	if len(buf) < reqLen {
		return 0, errors.New("md.Pack: buf too short to pack")
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

	buf[0] = md.version << 4
	if taskBits == 4 {
		buf[0] |= 3
	} else {
		buf[0] |= taskBits
	}
	buf[0] |= ((uint8(md.opLen) >> 2) - 1) << 3

	copy(buf[1:1+md.taskLen], md.ids.taskID)
	copy(buf[1+md.taskLen:1+md.taskLen+md.opLen], md.ids.opID)
	buf[1+md.taskLen+md.opLen] = md.flags

	return reqLen, nil
}

func (md *oboeMetadata) Unpack(data []byte) error {
	if md == nil {
		return errors.New("md.Unpack: nil md")
	}

	if len(data) == 0 { // no header to read
		return errors.New("md.Unpack: empty buf")
	}

	flag := data[0]
	var taskLen, opLen int
	var version uint8

	/* don't recognize this? */
	if (flag&maskVersion)>>4 != xtrCurrentVersion {
		return errors.New("md.Unpack: unrecognized X-Trace version")
	}
	version = (flag & maskVersion) >> 4

	taskLen = (int(flag&maskTaskIDLen) + 1) << 2
	if taskLen == 16 {
		taskLen = 20
	}
	opLen = ((int(flag&maskOpIDLen) >> 3) + 1) << 2

	/* do header lengths describe reality? */
	if (taskLen + opLen + 2) > len(data) {
		return errors.New("md.Unpack: wrong header length")
	}

	md.version = version
	md.taskLen = taskLen
	md.opLen = opLen

	md.ids.taskID = data[1 : 1+taskLen]
	md.ids.opID = data[1+taskLen : 1+taskLen+opLen]
	md.flags = data[1+taskLen+opLen]

	return nil
}

func (md *oboeMetadata) FromString(buf string) error {
	if md == nil {
		return errors.New("md.FromString: nil md")
	}

	ubuf := make([]byte, oboeMaxMetadataPackLen)

	// a hex string's length would be an even number
	if len(buf)%2 == 1 {
		return errors.New("md.FromString: hex not even")
	}

	// check if there are more hex bytes than we want
	if len(buf)/2 > oboeMaxMetadataPackLen {
		return errors.New("md.FromString: too many hex bytes")
	}

	// invalid hex?
	ret, err := hex.Decode(ubuf, []byte(buf))
	if ret != len(buf)/2 || err != nil {
		return errors.New("md.FromString: hex not valid")
	}
	ubuf = ubuf[:ret] // truncate buffer to fit decoded bytes
	return md.Unpack(ubuf)
}

func (md *oboeMetadata) ToString() (string, error) {
	if md == nil {
		return "", errors.New("md.ToString: nil md")
	}
	buf := make([]byte, 64)
	result, err := md.Pack(buf)
	if err != nil {
		return "", err
	}
	// encode as hex
	enc := make([]byte, 2*result)
	l := hex.Encode(enc, buf[:result])
	return strings.ToUpper(string(enc[:l])), nil
}

func (md *oboeMetadata) opString() string {
	enc := make([]byte, 2*md.opLen)
	l := hex.Encode(enc, md.ids.opID[:md.opLen])
	return strings.ToUpper(string(enc[:l]))
}

func (md *oboeMetadata) isSampled() bool {
	return md.flags&XTR_FLAGS_SAMPLED != 0
}

// A Context is an oboe context that may or not be tracing.
type Context interface {
	ReportEvent(label Label, layer string, args ...interface{}) error
	ReportEventMap(label Label, layer string, keys map[string]interface{}) error
	Copy() Context
	IsSampled() bool
	SetSampled(trace bool)
	SetEnabled(enabled bool)
	GetEnabled() bool
	SetTransactionName(name string)
	GetTransactionName() string
	MetadataString() string
	NewEvent(label Label, layer string, addCtxEdge bool) Event
	GetVersion() uint8
}

// A Event is an event that may or may not be tracing, created by a Context.
type Event interface {
	ReportContext(c Context, addCtxEdge bool, args ...interface{}) error
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
func (e *nullContext) Copy() Context                                         { return &nullContext{} }
func (e *nullContext) IsSampled() bool                                       { return false }
func (e *nullContext) SetSampled(trace bool)                                 {}
func (e *nullContext) SetEnabled(enabled bool)                               {}
func (e *nullContext) GetEnabled() bool                                      { return true }
func (e *nullContext) SetTransactionName(name string)                        {}
func (e *nullContext) GetTransactionName() string                            { return "" }
func (e *nullContext) MetadataString() string                                { return "" }
func (e *nullContext) NewEvent(l Label, y string, g bool) Event              { return &nullEvent{} }
func (e *nullContext) GetVersion() uint8                                     { return 0 }
func (e *nullEvent) ReportContext(c Context, g bool, a ...interface{}) error { return nil }
func (e *nullEvent) MetadataString() string                                  { return "" }

// NewNullContext returns a context that is not tracing.
func NewNullContext() Context { return &nullContext{} }

// newContext allocates a context with random metadata (for a new trace).
func newContext(sampled bool) Context {
	ctx := &oboeContext{txCtx: &transactionContext{enabled: true}}
	ctx.metadata.Init()
	if err := ctx.metadata.SetRandom(); err != nil {
		log.Infof("AppOptics rand.Read error: %v", err)
		return &nullContext{}
	}
	ctx.SetSampled(sampled)
	return ctx
}

func newContextFromMetadataString(mdstr string) (*oboeContext, error) {
	ctx := &oboeContext{txCtx: &transactionContext{enabled: true}}
	ctx.metadata.Init()
	err := ctx.metadata.FromString(mdstr)
	return ctx, err
}

// NewContext starts a trace for the provided URL, possibly continuing one, if
// mdStr is provided. Setting reportEntry will report an entry event before this
// function returns, calling cb if provided for additional KV pairs.
func NewContext(layer string, reportEntry bool, opts ContextOptions,
	cb func() KVMap) (ctx Context, ok bool, headers map[string]string) {
	traced := false
	addCtxEdge := false

	var SetHeaders = func(v string) {
		if headers == nil {
			headers = make(map[string]string)
		}
		headers["X-Trace-Options-Response"] = v
	}

	if opts.MdStr != "" {
		var err error
		if ctx, err = newContextFromMetadataString(opts.MdStr); err != nil {
			log.Debug("passed in x-trace seems invalid, ignoring")
		} else if ctx.GetVersion() != xtrCurrentVersion {
			log.Debug("passed in x-trace has wrong version, ignoring")
		} else if ctx.IsSampled() {
			traced = true
			addCtxEdge = true
		} else {
			setting, has := getSetting(layer)
			if !has {
				if opts.TriggerTrace {
					SetHeaders("force_trace=settings-not-available")
				}
				return ctx, false, headers // TODO: return true to attach headers here?
			}

			_, flags, _ := mergeURLSetting(setting, opts.URL)
			ctx.SetEnabled(flags.Enabled())
			if opts.XTraceOptions {
				if opts.TriggerTrace {
					SetHeaders("force_trace=ignored")
				} else {
					SetHeaders("ok")
				}
			}
			return ctx, true, headers
		}
	}

	if !traced {
		ctx = newContext(true)
	}

	decision := shouldTraceRequestWithURL(layer, traced, opts.URL, opts.TriggerTrace)
	if decision.trace {
		if reportEntry {
			var kvs map[string]interface{}
			if cb != nil {
				kvs = cb()
			}
			if len(kvs) == 0 {
				kvs = make(map[string]interface{})
			}
			kvs["SampleRate"] = decision.rate
			kvs["SampleSource"] = decision.source
			if _, ok = ctx.(*oboeContext); !ok {
				return &nullContext{}, false, headers
			}
			if err := ctx.(*oboeContext).reportEventMap(LabelEntry, layer, addCtxEdge, kvs); err != nil {
				return &nullContext{}, false, headers
			}
		}
		if opts.TriggerTrace {
			SetHeaders(decision.xTraceOptsRsp)
		}
		return ctx, true, headers
	}

	ctx.SetSampled(false)
	ctx.SetEnabled(decision.enabled)
	if opts.TriggerTrace {
		SetHeaders(decision.xTraceOptsRsp)
	}
	return ctx, true, headers
}

func (ctx *oboeContext) Copy() Context {
	md := oboeMetadata{}
	md.Init()
	copy(md.ids.taskID, ctx.metadata.ids.taskID)
	copy(md.ids.opID, ctx.metadata.ids.opID)
	md.flags = ctx.metadata.flags
	return &oboeContext{metadata: md, txCtx: ctx.txCtx}
}
func (ctx *oboeContext) IsSampled() bool { return ctx.metadata.isSampled() }

func (ctx *oboeContext) SetSampled(trace bool) {
	if trace {
		ctx.metadata.flags |= XTR_FLAGS_SAMPLED // set sampled bit
	} else {
		ctx.metadata.flags ^= XTR_FLAGS_SAMPLED // clear sampled bit
	}
}

func (ctx *oboeContext) SetEnabled(enabled bool) {
	ctx.txCtx.enabled = enabled
}

func (ctx *oboeContext) GetEnabled() bool {
	return ctx.txCtx.enabled
}

func (ctx *oboeContext) SetTransactionName(name string) {
	ctx.txCtx.Lock()
	defer ctx.txCtx.Unlock()
	ctx.txCtx.name = name
}

func (ctx *oboeContext) GetTransactionName() string {
	ctx.txCtx.RLock()
	defer ctx.txCtx.RUnlock()
	return ctx.txCtx.name
}

func (ctx *oboeContext) newEvent(label Label, layer string) (*event, error) {
	return newEvent(&ctx.metadata, label, layer)
}

func (ctx *oboeContext) NewEvent(label Label, layer string, addCtxEdge bool) Event {
	e, err := newEvent(&ctx.metadata, label, layer)
	if err != nil {
		return &nullEvent{}
	}
	if addCtxEdge {
		e.AddEdge(ctx)
	}
	return e
}

func (ctx *oboeContext) GetVersion() uint8 {
	return ctx.metadata.version
}

// Create and report and event using a map of KVs
func (ctx *oboeContext) ReportEventMap(label Label, layer string, keys map[string]interface{}) error {
	return ctx.reportEventMap(label, layer, true, keys)
}

func (ctx *oboeContext) reportEventMap(label Label, layer string, addCtxEdge bool, keys map[string]interface{}) error {
	var args []interface{}
	for k, v := range keys {
		args = append(args, k)
		args = append(args, v)
	}
	return ctx.reportEvent(label, layer, addCtxEdge, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *oboeContext) ReportEvent(label Label, layer string, args ...interface{}) error {
	return ctx.reportEvent(label, layer, true, args...)
}

// Create and report an event using KVs from variadic args
func (ctx *oboeContext) reportEvent(label Label, layer string, addCtxEdge bool, args ...interface{}) error {
	// create new event from context
	e, err := ctx.newEvent(label, layer)
	if err != nil { // error creating event (e.g. couldn't init random IDs)
		return err
	}
	return ctx.report(e, addCtxEdge, args...)
}

// report an event using KVs from variadic args
func (ctx *oboeContext) report(e *event, addCtxEdge bool, args ...interface{}) error {
	for i := 0; i+1 < len(args); i += 2 {
		if err := e.AddKV(args[i], args[i+1]); err != nil {
			return err
		}
	}
	if addCtxEdge {
		e.AddEdge(ctx)
	}
	// report event
	return e.Report(ctx)
}

func (ctx *oboeContext) MetadataString() string { return ctx.metadata.String() }

// String returns a hex string representation
func (md *oboeMetadata) String() string {
	mdStr, _ := md.ToString()
	return mdStr
}
