package traceview

import (
	"math"
)

type bson_buffer struct {
	buf []byte
}

// Conforms to C interface to simplify port
// XXX: convert to something more go-like

func bson_buffer_init(b *bson_buffer) {
	b.buf = make([]byte, 0, 4)
	b.reserveInt32()
}

func bson_buffer_finish(b *bson_buffer) {
	b.addBytes(0)
	b.setInt32(0, int32(len(b.buf)))
}

func bson_append_string(b *bson_buffer, k, v string) {
	b.addElemName('\x02', k)
	b.addStr(v)
}

func bson_append_int(b *bson_buffer, k string, v int) {
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		bson_append_int32(b, k, int32(v))
	} else {
		bson_append_int64(b, k, int64(v))
	}
}

func bson_append_int32(b *bson_buffer, k string, v int32) {
	b.addElemName('\x10', k)
	b.addInt32(v)
}

func bson_append_int64(b *bson_buffer, k string, v int64) {
	b.addElemName('\x12', k)
	b.addInt64(v)
}

// Based on https://github.com/go-mgo/mgo/blob/v2/bson/encodb.go
// --------------------------------------------------------------------------
// Marshaling of elements in a document.

func (b *bson_buffer) addElemName(kind byte, name string) {
	b.addBytes(kind)
	b.addBytes([]byte(name)...)
	b.addBytes(0)
}

// Marshaling of base types.

func (b *bson_buffer) addBinary(subtype byte, v []byte) {
	if subtype == 0x02 {
		// Wonder how that brilliant idea came to lifb. Obsolete, luckily.
		b.addInt32(int32(len(v) + 4))
		b.addBytes(subtype)
		b.addInt32(int32(len(v)))
	} else {
		b.addInt32(int32(len(v)))
		b.addBytes(subtype)
	}
	b.addBytes(v...)
}

func (b *bson_buffer) addStr(v string) {
	b.addInt32(int32(len(v) + 1))
	b.addCStr(v)
}

func (b *bson_buffer) addCStr(v string) {
	b.addBytes([]byte(v)...)
	b.addBytes(0)
}

func (b *bson_buffer) reserveInt32() (pos int) {
	pos = len(b.buf)
	b.addBytes(0, 0, 0, 0)
	return pos
}

func (b *bson_buffer) setInt32(pos int, v int32) {
	b.buf[pos+0] = byte(v)
	b.buf[pos+1] = byte(v >> 8)
	b.buf[pos+2] = byte(v >> 16)
	b.buf[pos+3] = byte(v >> 24)
}

func (b *bson_buffer) addInt32(v int32) {
	u := uint32(v)
	b.addBytes(byte(u), byte(u>>8), byte(u>>16), byte(u>>24))
}

func (b *bson_buffer) addInt64(v int64) {
	u := uint64(v)
	b.addBytes(byte(u), byte(u>>8), byte(u>>16), byte(u>>24),
		byte(u>>32), byte(u>>40), byte(u>>48), byte(u>>56))
}

func (b *bson_buffer) addFloat64(v float64) {
	b.addInt64(int64(math.Float64bits(v)))
}

func (b *bson_buffer) addBytes(v ...byte) {
	b.buf = append(b.buf, v...)
}
