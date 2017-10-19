package hdrhist

import "errors"

/*
This file was ported from the Java source of HdrHistogram written
by Gil Tene. See https://hdrhistogram.github.io/HdrHistogram/.

This files provides encoding and decoding functions for writing and
reading ZigZag-encoded LEB128-64b9B-variant (Little Endian Base 128)
values to/from a byte slice. LEB128's variable length encoding
provides for using a smaller nuber of bytes for smaller values, and
the use of ZigZag encoding allows small (closer to zero) negative
values to use fewer bytes. Details on both LEB128 and ZigZag can be
readily found elsewhere.

The LEB128-64b9B-variant encoding used here diverges from the
"original" LEB128 as it extends to 64 bit values: In the original
LEB128, a 64 bit value can take up to 10 bytes in the stream, where
this variant's encoding of a 64 bit values will max out at 9 bytes.

As such, this encoder/decoder should NOT be used for encoding or
decoding "standard" LEB128 formats (e.g. Google Protocol Buffers).
*/

func encodeZigZag(i int64) []byte {
	b := make([]byte, 0, 8)
	value := uint64((i << 1) ^ (i >> 63))
	if value>>7 == 0 {
		b = append(b, byte(value))
	} else {
		b = append(b, byte((value&0x7F)|0x80))
		if uint64(value)>>14 == 0 {
			b = append(b, byte(value>>7))
		} else {
			b = append(b, byte(value>>7|0x80))
			if value>>21 == 0 {
				b = append(b, byte(value>>14))
			} else {
				b = append(b, byte(value>>14|0x80))
				if value>>28 == 0 {
					b = append(b, byte(value>>21))
				} else {
					b = append(b, byte(value>>21|0x80))
					if value>>35 == 0 {
						b = append(b, byte(value>>28))
					} else {
						b = append(b, byte(value>>28|0x80))
						if value>>42 == 0 {
							b = append(b, byte(value>>35))
						} else {
							b = append(b, byte(value>>35|0x80))
							if value>>49 == 0 {
								b = append(b, byte(value>>42))
							} else {
								b = append(b, byte(value>>42|0x80))
								if value>>56 == 0 {
									b = append(b, byte(value>>49))
								} else {
									b = append(b, byte(value>>49|0x80))
									b = append(b, byte(value>>56))
								}
							}
						}
					}
				}
			}
		}
	}
	return b
}

func decodeZigZagUnsafe(b []byte) (int64, int) {
	v := int64(b[0])
	vlen := 1
	value := v & 0x7F
	if (v & 0x80) != 0 {
		v = int64(b[1])
		vlen++
		value |= (v & 0x7F) << 7
		if (v & 0x80) != 0 {
			v = int64(b[2])
			vlen++
			value |= (v & 0x7F) << 14
			if (v & 0x80) != 0 {
				v = int64(b[3])
				vlen++
				value |= (v & 0x7F) << 21
				if (v & 0x80) != 0 {
					v = int64(b[4])
					vlen++
					value |= (v & 0x7F) << 28
					if (v & 0x80) != 0 {
						v = int64(b[5])
						vlen++
						value |= (v & 0x7F) << 35
						if (v & 0x80) != 0 {
							v = int64(b[6])
							vlen++
							value |= (v & 0x7F) << 42
							if (v & 0x80) != 0 {
								v = int64(b[7])
								vlen++
								value |= (v & 0x7F) << 49
								if (v & 0x80) != 0 {
									v = int64(b[9])
									vlen++
									value |= v << 56
								}
							}
						}
					}
				}
			}
		}
	}
	return int64((uint64(value) >> 1) ^ uint64(-(value & 1))), vlen
}

// decodeZigZag safely catch bounds errors due to
// missing data.
func decodeZigZag(b []byte) (v int64, vlen int, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.New("got incomplete data")
		}
	}()

	v, vlen = decodeZigZagUnsafe(b)
	return v, vlen, nil
}
