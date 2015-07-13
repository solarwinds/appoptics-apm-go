package traceview

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

/*
#cgo pkg-config: openssl
#cgo LDFLAGS: -loboe
#include <stdlib.h>
#include <oboe/oboe.h>
*/
import "C"

const OBOE_METADATA_STRING_LEN = 58
const MASK_TASK_ID_LEN = 0x03
const MASK_OP_ID_LEN = 0x08
const MASK_HAS_OPTIONS = 0x04
const MASK_VERSION = 0xF0

const XTR_CURRENT_VERSION = 1

const OBOE_MAX_TASK_ID_LEN = 20
const OBOE_MAX_OP_ID_LEN = 8
const OBOE_MAX_METADATA_PACK_LEN = 512

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

	md.task_len = OBOE_MAX_TASK_ID_LEN
	md.op_len = OBOE_MAX_OP_ID_LEN
	md.ids.task_id = make([]byte, OBOE_MAX_TASK_ID_LEN)
	md.ids.op_id = make([]byte, OBOE_MAX_TASK_ID_LEN)

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

	buf[0] = XTR_CURRENT_VERSION << 4
	if task_bits == 4 {
		buf[0] |= 3
	} else {
		buf[0] |= task_bits
	}
	buf[0] |= ((uint8(md.op_len) >> 2) - 1) << 3

	copy(buf[1:1+md.task_len], md.ids.task_id)
	copy(buf[md.task_len:md.task_len+md.op_len], md.ids.op_id)

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
	if (flag&MASK_VERSION)>>4 != XTR_CURRENT_VERSION {
		return -2
	}

	task_len = (int(flag&MASK_TASK_ID_LEN) + 1) << 2
	if task_len == 16 {
		task_len = 20
	}
	op_len = ((int(flag&MASK_OP_ID_LEN) >> 3) + 1) << 2

	/* do header lengths describe reality? */
	if (task_len + op_len + 1) != len(data) {
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

	ubuf := make([]byte, OBOE_MAX_METADATA_PACK_LEN)

	// a hex string's length would be an even number
	if len(buf)%2 == 1 {
		return -1
	}

	// check if there are more hex bytes than we want
	if len(buf)/2 > OBOE_MAX_METADATA_PACK_LEN {
		return -1
	}

	// invalid hex?
	ret, err := hex.Decode(ubuf, []byte(buf))
	if ret != len(buf)/2 || err != nil {
		return -1
	}

	return oboe_metadata_unpack(md, ubuf)
}

func oboe_metadata_tostr(md *oboe_metadata_t) (string, error) {
	if md == nil {
		return "", errors.New("invalid metadata")
	}

	buf := make([]byte, 64)
	result := oboe_metadata_pack(md, buf)
	if result < 0 {
		return "", errors.New("unable to pack metadata")
	}

	/* result is # of packed bytes */
	if !(2*result < len(buf)) { // hex repr of md is 2*(# of packed bytes)
		return "", errors.New("buffer too small")
	}
	enc := make([]byte, 2*result)
	len := hex.Encode(enc, buf[:result-1])
	return string(enc[:len]), nil
}

// Allocates context with random metadata (new trace)
func NewContext() *Context {
	ctx := &Context{}
	oboe_metadata_init(&ctx.metadata)
	oboe_metadata_random(&ctx.metadata)
	return ctx
}

// Allocates context with existing metadata (for continuing traces)
func NewContextFromMetaDataString(mdstr string) *Context {
	ctx := &Context{}
	oboe_metadata_init(&ctx.metadata)
	oboe_metadata_fromstr(&ctx.metadata, mdstr)
	return ctx
}

func (ctx *Context) NewEvent(label Label, layer string) *Event {
	return NewEvent(&ctx.metadata, label, layer)
}

func (ctx *Context) String() string {
	return fmt.Sprintf("Context: %s", metadataString(&ctx.metadata))
}

// converts metadata (*oboe_metadata_t) to a string representation
func metadataString(metadata *oboe_metadata_t) string {
	md_str, _ := oboe_metadata_tostr(metadata)
	// XXX: error check?
	return md_str
}
