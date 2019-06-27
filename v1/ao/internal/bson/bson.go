// Copyright (C) 2016 Librato, Inc. All rights reserved.

package bson

import "math"

type Buffer struct {
	buf []byte
}

func (b *Buffer) GetBuf() []byte { return b.buf }

// NewBuffer creates a new bson buffer
func NewBuffer() *Buffer {
	var bbuf = &Buffer{}
	bbuf.Init()
	return bbuf
}

func WithBuf(buf []byte) *Buffer {
	return &Buffer{buf: buf}
}

// Conforms to C interface to simplify port

func (b *Buffer) Init() {
	b.buf = make([]byte, 0, 4)
	b.reserveInt32()
}

func (b *Buffer) Finish() {
	b.addBytes(0)
	b.setInt32(0, int32(len(b.buf)))
}

func (b *Buffer) AppendString(k, v string) {
	b.addElemName('\x02', k)
	b.addStr(v)
}

func (b *Buffer) AppendBinary(k string, v []byte) {
	b.addElemName('\x05', k)
	b.addBinary(v)
}

func (b *Buffer) AppendInt(k string, v int) {
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		b.AppendInt32(k, int32(v))
	} else {
		b.AppendInt64(k, int64(v))
	}
}

func (b *Buffer) AppendInt32(k string, v int32) {
	b.addElemName('\x10', k)
	b.addInt32(v)
}

func (b *Buffer) AppendInt64(k string, v int64) {
	b.addElemName('\x12', k)
	b.addInt64(v)
}

func (b *Buffer) AppendFloat64(k string, v float64) {
	b.addElemName('\x01', k)
	b.addFloat64(v)
}

func (b *Buffer) AppendBool(k string, v bool) {
	b.addElemName('\x08', k)
	if v {
		b.addBytes(1)
	} else {
		b.addBytes(0)
	}
}

func (b *Buffer) AppendStartObject(k string) (start int) {
	b.addElemName('\x03', k)
	start = b.reserveInt32()
	return
}

func (b *Buffer) AppendStartArray(k string) (start int) {
	b.addElemName('\x04', k)
	start = b.reserveInt32()
	return
}

func (b *Buffer) AppendFinishObject(start int) {
	b.addBytes(0)
	b.setInt32(start, int32(len(b.buf)-start))
}

// Based on https://github.com/go-mgo/mgo/blob/v2/bson/encode.go
// --------------------------------------------------------------------------
// Marshaling of elements in a document.

func (b *Buffer) addElemName(kind byte, name string) {
	b.addBytes(kind)
	b.addBytes([]byte(name)...)
	b.addBytes(0)
}

// Marshaling of base types.

func (b *Buffer) addBinary(v []byte) {
	subtype := byte(0) // don't use obsolete 0x02 subtype
	b.addInt32(int32(len(v)))
	b.addBytes(subtype)
	b.addBytes(v...)
}

func (b *Buffer) addStr(v string) {
	b.addInt32(int32(len(v) + 1))
	b.addCStr(v)
}

func (b *Buffer) addCStr(v string) {
	b.addBytes([]byte(v)...)
	b.addBytes(0)
}

func (b *Buffer) reserveInt32() (pos int) {
	pos = len(b.buf)
	b.addBytes(0, 0, 0, 0)
	return pos
}

func (b *Buffer) setInt32(pos int, v int32) {
	b.buf[pos+0] = byte(v)
	b.buf[pos+1] = byte(v >> 8)
	b.buf[pos+2] = byte(v >> 16)
	b.buf[pos+3] = byte(v >> 24)
}

func (b *Buffer) addInt32(v int32) {
	u := uint32(v)
	b.addBytes(byte(u), byte(u>>8), byte(u>>16), byte(u>>24))
}

func (b *Buffer) addInt64(v int64) {
	u := uint64(v)
	b.addBytes(byte(u), byte(u>>8), byte(u>>16), byte(u>>24),
		byte(u>>32), byte(u>>40), byte(u>>48), byte(u>>56))
}

func (b *Buffer) addFloat64(v float64) {
	b.addInt64(int64(math.Float64bits(v)))
}

func (b *Buffer) addBytes(v ...byte) {
	b.buf = append(b.buf, v...)
}
