// Copyright (C) 2016 Librato, Inc. All rights reserved.

// Package traceview provides a low-level API for creating and reporting events for
// distributed tracing with TraceView.
package traceview

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
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
		return fmt.Errorf("Key %v (type %T) not a string", k, k)
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
		// silently skip unsupported value type
		if debugLog {
			log.Printf("Unrecognized Event key %v val %v", k, v)
		}
	}
	return nil
}

// Reports event using specified Reporter
func (e *event) ReportUsing(c *oboeContext, r reporter) error { return reportEvent(r, c, e) }

// Reports event using default (UDP) Reporter
func (e *event) Report(c *oboeContext) error { return e.ReportUsing(c, globalReporter()) }

// Report event using Context interface
func (e *event) ReportContext(c Context, addCtxEdge bool, args ...interface{}) error {
	if ctx, ok := c.(*oboeContext); ok {
		return ctx.report(e, addCtxEdge, args...)
	}
	return nil
}

// Returns Metadata string (X-Trace header)
func (e *event) MetadataString() string { return e.metadata.String() }
