// Copyright (C) 2016 Librato, Inc. All rights reserved.

// Package reporter provides a low-level API for creating and reporting events for
// distributed tracing with AppOptics.
package reporter

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
)

type event struct {
	metadata oboeMetadata
	bbuf     bsonBuffer
}

// Label is a required event attribute.
type Label string

// Labels used for reporting events for Layer and Profile spans.
const (
	LabelEntry        = "entry"
	LabelExit         = "exit"
	LabelInfo         = "info"
	LabelError        = "error"
	LabelProfileEntry = "profile_entry"
	LabelProfileExit  = "profile_exit"
	EdgeKey           = "Edge"
)

const (
	eventHeader = "1"
)

// enums used by sampling and tracing settings
type tracingMode int
type settingType int
type settingFlag uint16
type sampleSource int

// tracing modes
const (
	TRACE_DISABLED tracingMode = iota // disable tracing, will neither start nor continue traces
	TRACE_ENABLED                     // perform sampling every inbound request for tracing
)

// setting types
const (
	TYPE_DEFAULT settingType = iota // default setting which serves as a fallback if no other settings found
	TYPE_LAYER                      // layer specific settings
)

// setting flags
const (
	FLAG_OK                    settingFlag = 0x0
	FLAG_INVALID               settingFlag = 0x1
	FLAG_OVERRIDE              settingFlag = 0x2
	FLAG_SAMPLE_START          settingFlag = 0x4
	FLAG_SAMPLE_THROUGH        settingFlag = 0x8
	FLAG_SAMPLE_THROUGH_ALWAYS settingFlag = 0x10
)

// source of the sample value
const (
	SAMPLE_SOURCE_NONE    sampleSource = 0
	SAMPLE_SOURCE_FILE    sampleSource = 1
	SAMPLE_SOURCE_DEFAULT sampleSource = 2
	SAMPLE_SOURCE_LAYER   sampleSource = 3
)

const (
	maxSamplingRate = config.MaxSampleRate
)

func (st settingType) toSampleSource() sampleSource {
	var source sampleSource
	switch st {
	case TYPE_DEFAULT:
		source = SAMPLE_SOURCE_DEFAULT
	case TYPE_LAYER:
		source = SAMPLE_SOURCE_LAYER
	default:
		source = SAMPLE_SOURCE_NONE
	}
	return source
}

// newTracingMode creates a tracing mode object from a string
func newTracingMode(mode config.TracingMode) tracingMode {
	switch mode {
	case config.DisabledTracingMode:
		return TRACE_DISABLED
	case config.EnabledTracingMode:
	default:
	}
	return TRACE_ENABLED
}

func (tm tracingMode) toFlags() settingFlag {
	switch tm {
	case TRACE_ENABLED:
		return FLAG_SAMPLE_START | FLAG_SAMPLE_THROUGH_ALWAYS
	case TRACE_DISABLED:
	default:
	}
	return FLAG_OK
}

func oboeEventInit(evt *event, md *oboeMetadata) error {
	if evt == nil || md == nil {
		return errors.New("oboeEventInit got nil args")
	}

	// Metadata initialization
	evt.metadata.Init()

	evt.metadata.taskLen = md.taskLen
	evt.metadata.opLen = md.opLen

	copy(evt.metadata.ids.taskID, md.ids.taskID)
	if err := evt.metadata.SetRandomOpID(); err != nil {
		return err
	}
	evt.metadata.flags = md.flags

	// Buffer initialization
	bsonBufferInit(&evt.bbuf)

	// Copy header to buffer
	bsonAppendString(&evt.bbuf, "_V", eventHeader)

	// Pack metadata
	mdStr, err := evt.metadata.ToString()
	if err != nil {
		return err
	}
	bsonAppendString(&evt.bbuf, "X-Trace", mdStr)
	return nil
}

func newEvent(md *oboeMetadata, label Label, layer string) (*event, error) {
	e := &event{}
	if err := oboeEventInit(e, md); err != nil {
		return nil, err
	}
	e.addLabelLayer(label, layer)
	return e, nil
}

func (e *event) addLabelLayer(label Label, layer string) {
	e.AddString("Label", string(label))
	if layer != "" {
		e.AddString("Layer", layer)
	}
}

// Adds string key/value to event. BSON strings are assumed to be Unicode.
func (e *event) AddString(key, value string) { bsonAppendString(&e.bbuf, key, value) }

// Adds a binary buffer as a key/value to this event. This uses a binary-safe BSON buffer type.
func (e *event) AddBinary(key string, value []byte) { bsonAppendBinary(&e.bbuf, key, value) }

// Adds int key/value to event
func (e *event) AddInt(key string, value int) { bsonAppendInt(&e.bbuf, key, value) }

// Adds int64 key/value to event
func (e *event) AddInt64(key string, value int64) { bsonAppendInt64(&e.bbuf, key, value) }

// Adds int32 key/value to event
func (e *event) AddInt32(key string, value int32) { bsonAppendInt32(&e.bbuf, key, value) }

// Adds float32 key/value to event
func (e *event) AddFloat32(key string, value float32) { bsonAppendFloat64(&e.bbuf, key, float64(value)) }

// Adds float64 key/value to event
func (e *event) AddFloat64(key string, value float64) { bsonAppendFloat64(&e.bbuf, key, value) }

// Adds float key/value to event
func (e *event) AddBool(key string, value bool) { bsonAppendBool(&e.bbuf, key, value) }

// Adds edge (reference to previous event) to event
func (e *event) AddEdge(ctx *oboeContext) { bsonAppendString(&e.bbuf, EdgeKey, ctx.metadata.opString()) }

func (e *event) AddEdgeFromMetadataString(mdstr string) {
	var md oboeMetadata
	md.Init()
	err := md.FromString(mdstr)
	// only add Edge if metadata references same trace as ours
	if err == nil && bytes.Equal(e.metadata.ids.taskID, md.ids.taskID) {
		bsonAppendString(&e.bbuf, EdgeKey, md.opString())
	}
}

// Add any key/value to event. May not add KV if key or value is invalid. Used to facilitate
// reporting variadic args.
func (e *event) AddKV(key, value interface{}) error {
	// load key name
	k, isStr := key.(string)
	if !isStr {
		return fmt.Errorf("key %v (type %T) not a string", k, k)
	}
	// load value and add KV to event
	switch v := value.(type) {
	case string:
		if k == EdgeKey {
			e.AddEdgeFromMetadataString(v)
		} else {
			e.AddString(k, v)
		}
	case []byte:
		e.AddBinary(k, v)
	case int:
		e.AddInt(k, v)
	case int64:
		e.AddInt64(k, v)
	case int32:
		e.AddInt32(k, v)
	case uint:
		if v <= math.MaxInt64 {
			e.AddInt64(k, int64(v))
		}
	case uint64:
		if v <= math.MaxInt64 {
			e.AddInt64(k, int64(v))
		}
	case uint32:
		e.AddInt64(k, int64(v))
	case float32:
		e.AddFloat32(k, v)
	case float64:
		e.AddFloat64(k, v)
	case bool:
		e.AddBool(k, v)
	case *oboeContext:
		if k == EdgeKey {
			e.AddEdge(v)
		}
	case sampleSource:
		e.AddInt(k, int(v))

	// allow reporting of pointers to basic types as well (for delayed evaluation)
	case *string:
		if v != nil {
			if k == EdgeKey {
				e.AddEdgeFromMetadataString(*v)
			} else {
				e.AddString(k, *v)
			}
		}
	case *[]byte:
		if v != nil {
			e.AddBinary(k, *v)
		}
	case *int:
		if v != nil {
			e.AddInt(k, *v)
		}
	case *int64:
		if v != nil {
			e.AddInt64(k, *v)
		}
	case *int32:
		if v != nil {
			e.AddInt32(k, *v)
		}
	case *uint:
		if v != nil {
			if *v <= math.MaxInt64 {
				e.AddInt64(k, int64(*v))
			}
		}
	case *uint64:
		if v != nil {
			if *v <= math.MaxInt64 {
				e.AddInt64(k, int64(*v))
			}
		}
	case *uint32:
		if v != nil {
			e.AddInt64(k, int64(*v))
		}
	case *float32:
		if v != nil {
			e.AddFloat32(k, *v)
		}
	case *float64:
		if v != nil {
			e.AddFloat64(k, *v)
		}
	case *bool:
		if v != nil {
			e.AddBool(k, *v)
		}
	default:
		log.Debugf("Ignoring unrecognized Event key %v val %v valType %T", k, v, v)
	}
	return nil
}

// Reports event using specified Reporter
func (e *event) ReportUsing(c *oboeContext, r reporter, channel reporterChannel) error {
	if channel == EVENTS {
		if e.metadata.isSampled() {
			return r.reportEvent(c, e)
		}
	} else if channel == METRICS {
		return r.reportStatus(c, e)
	}
	return nil
}

// Reports event using default Reporter
func (e *event) Report(c *oboeContext) error       { return e.ReportUsing(c, globalReporter, EVENTS) }
func (e *event) ReportStatus(c *oboeContext) error { return e.ReportUsing(c, globalReporter, METRICS) }

// Report event using Context interface
func (e *event) ReportContext(c Context, addCtxEdge bool, args ...interface{}) error {
	if ctx, ok := c.(*oboeContext); ok {
		return ctx.report(e, addCtxEdge, args...)
	}
	return nil
}

// Returns Metadata string (X-Trace header)
func (e *event) MetadataString() string { return e.metadata.String() }
