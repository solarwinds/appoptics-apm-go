// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import "math"

type bsonBuffer struct {
	buf []byte
}

// Conforms to C interface to simplify port

func bsonBufferInit(b *bsonBuffer) {
	b.buf = make([]byte, 0, 4)
	b.reserveInt32()
}

func bsonBufferFinish(b *bsonBuffer) {
	b.addBytes(0)
	b.setInt32(0, int32(len(b.buf)))
}

func bsonAppendString(b *bsonBuffer, k, v string) {
	b.addElemName('\x02', k)
	b.addStr(v)
}

func bsonAppendBinary(b *bsonBuffer, k string, v []byte) {
	b.addElemName('\x05', k)
	b.addBinary(v)
}

func bsonAppendInt(b *bsonBuffer, k string, v int) {
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		bsonAppendInt32(b, k, int32(v))
	} else {
		bsonAppendInt64(b, k, int64(v))
	}
}

func bsonAppendInt32(b *bsonBuffer, k string, v int32) {
	b.addElemName('\x10', k)
	b.addInt32(v)
}

func bsonAppendInt64(b *bsonBuffer, k string, v int64) {
	b.addElemName('\x12', k)
	b.addInt64(v)
}

func bsonAppendFloat64(b *bsonBuffer, k string, v float64) {
	b.addElemName('\x01', k)
	b.addFloat64(v)
}

func bsonAppendBool(b *bsonBuffer, k string, v bool) {
	b.addElemName('\x08', k)
	if v {
		b.addBytes(1)
	} else {
		b.addBytes(0)
	}
}

func bsonAppendStartObject(b *bsonBuffer, k string) (start int) {
	b.addElemName('\x03', k)
	start = b.reserveInt32()
	return
}

func bsonAppendFinishObject(b *bsonBuffer, start int) {
	b.addBytes(0)
	b.setInt32(start, int32(len(b.buf)-start))
}

// Based on https://github.com/go-mgo/mgo/blob/v2/bson/encode.go
// --------------------------------------------------------------------------
// Marshaling of elements in a document.

func (b *bsonBuffer) addElemName(kind byte, name string) {
	b.addBytes(kind)
	b.addBytes([]byte(name)...)
	b.addBytes(0)
}

// Marshaling of base types.

func (b *bsonBuffer) addBinary(v []byte) {
	subtype := byte(0) // don't use obsolete 0x02 subtype
	b.addInt32(int32(len(v)))
	b.addBytes(subtype)
	b.addBytes(v...)
}

func (b *bsonBuffer) addStr(v string) {
	b.addInt32(int32(len(v) + 1))
	b.addCStr(v)
}

func (b *bsonBuffer) addCStr(v string) {
	b.addBytes([]byte(v)...)
	b.addBytes(0)
}

func (b *bsonBuffer) reserveInt32() (pos int) {
	pos = len(b.buf)
	b.addBytes(0, 0, 0, 0)
	return pos
}

func (b *bsonBuffer) setInt32(pos int, v int32) {
	b.buf[pos+0] = byte(v)
	b.buf[pos+1] = byte(v >> 8)
	b.buf[pos+2] = byte(v >> 16)
	b.buf[pos+3] = byte(v >> 24)
}

func (b *bsonBuffer) addInt32(v int32) {
	u := uint32(v)
	b.addBytes(byte(u), byte(u>>8), byte(u>>16), byte(u>>24))
}

func (b *bsonBuffer) addInt64(v int64) {
	u := uint64(v)
	b.addBytes(byte(u), byte(u>>8), byte(u>>16), byte(u>>24),
		byte(u>>32), byte(u>>40), byte(u>>48), byte(u>>56))
}

func (b *bsonBuffer) addFloat64(v float64) {
	b.addInt64(int64(math.Float64bits(v)))
}

func (b *bsonBuffer) addBytes(v ...byte) {
	b.buf = append(b.buf, v...)
}
