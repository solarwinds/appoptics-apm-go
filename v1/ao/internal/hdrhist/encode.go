package hdrhist

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

func EncodeCompressed(h *Hist) ([]byte, error) {
	var buf bytes.Buffer
	b64w := base64.NewEncoder(base64.StdEncoding, &buf)
	if err := encodeCompressed(h, b64w, h.Max()); err != nil {
		b64w.Close()
		return nil, errors.Wrap(err, "unable to encode histogram")
	}
	b64w.Close() // DO NOT defer this close, otherwise that could prevent bytes from getting flushed
	return buf.Bytes(), nil
}

func encodeCompressed(h *Hist, w io.Writer, histMax int64) error {
	const compressedEncodingCookie = compressedEncodingV2CookieBase | 0x10
	var buf bytes.Buffer

	var cookie int32 = compressedEncodingCookie
	binary.Write(&buf, binary.BigEndian, cookie)
	buf.WriteString("\x00\x00\x00\x00")
	preCompressed := buf.Len()
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	encodeInto(h, zw, histMax) // won't error, not io device
	zw.Close()
	binary.BigEndian.PutUint32(buf.Bytes()[4:], uint32(buf.Len()-preCompressed))

	_, err := buf.WriteTo(w)
	return errors.Wrap(err, "unable to write compressed hist")
}

func encodeInto(h *Hist, w io.Writer, max int64) error {
	const encodingCookie = encodingV2CookieBase | 0x10

	importantLen := h.b.countsIndex(max) + 1
	var buf bytes.Buffer
	var cookie int32 = encodingCookie
	binary.Write(&buf, binary.BigEndian, cookie)
	buf.WriteString("\x00\x00\x00\x00") // length will go here
	buf.WriteString("\x00\x00\x00\x00") // normalizing index offset
	cfg := h.Config()
	binary.Write(&buf, binary.BigEndian, int32(cfg.SigFigs))
	binary.Write(&buf, binary.BigEndian, int64(cfg.LowestDiscernible))
	binary.Write(&buf, binary.BigEndian, int64(cfg.HighestTrackable))
	// int to double conversion ratio
	buf.WriteString("\x3f\xf0\x00\x00\x00\x00\x00\x00")
	payloadStart := buf.Len()
	fillBuffer(&buf, h, importantLen)
	binary.BigEndian.PutUint32(buf.Bytes()[4:], uint32(buf.Len()-payloadStart))
	_, err := buf.WriteTo(w)
	return errors.Wrap(err, "unable to write uncompressed hist")
}

func fillBuffer(buf *bytes.Buffer, h *Hist, n int) {
	srci := 0
	for srci < n {
		// V2 format uses a ZigZag LEB128-64b9B encoded int64.
		// Positive values are counts, negative values indicate
		// a run zero counts of that length.
		c := h.b.counts[srci]
		srci++
		if c < 0 {
			panic(errors.Errorf(
				"can't encode hist with negative counts (count: %d, idx: %d, value range: [%d, %d])",
				c,
				srci,
				h.b.lowestEquiv(h.b.valueFor(srci)),
				h.b.highestEquiv(h.b.valueFor(srci)),
			))
		}

		// count zeros run length
		zerosCount := int64(0)
		if c == 0 {
			zerosCount++
			for srci < n && h.b.counts[srci] == 0 {
				zerosCount++
				srci++
			}
		}
		if zerosCount > 1 {
			buf.Write(encodeZigZag(-zerosCount))
		} else {
			buf.Write(encodeZigZag(c))
		}
	}
}
