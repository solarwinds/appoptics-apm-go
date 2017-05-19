package ottv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracelytics/go-traceview/v1/tv"
	"github.com/tracelytics/go-traceview/v1/tv/internal/traceview"
)

type badReader struct{}
type shortBadReader struct{}

func (r badReader) Read(p []byte) (n int, err error)      { return 0, errors.New("read error") }
func (r shortBadReader) Read(p []byte) (n int, err error) { return 1, errors.New("read error") }

type badLimitedReader struct { // based on io.LimitedReader
	R        io.Reader // underlying reader
	N        int64     // max bytes remaining
	errorEOF bool      // whether to return an extra byte and an error when finished
}

func (l *badLimitedReader) Read(p []byte) (n int, err error) {
	if l.N <= 0 {
		if l.errorEOF {
			return 1, errors.New("read EOF error")
		}
		return 0, io.EOF
	}
	if int64(len(p)) > l.N {
		p = p[0:l.N]
	}
	n, err = l.R.Read(p)
	l.N -= int64(n)
	return n, errors.New("read error")
}

func TestBinaryExtract(t *testing.T) {
	_ = traceview.SetTestReporter()
	tr := NewTracer()

	// test invalid wire data
	invalidWire := bytes.NewBufferString("!!!")
	sc, err := tr.Extract(opentracing.Binary, invalidWire)
	assert.Equal(t, err, opentracing.ErrSpanContextCorrupted)
	assert.Nil(t, sc)

	// test bad reader being passed to Extract
	sc, err = tr.Extract(opentracing.Binary, badReader{})
	assert.Equal(t, err, opentracing.ErrSpanContextCorrupted)
	assert.Nil(t, sc)

	length := uint32(3)
	// test reader that returns length header, but then fails
	carrier := new(bytes.Buffer)
	err = binary.Write(carrier, binary.BigEndian, &length) // write length header
	assert.NoError(t, err)
	sc, err = tr.Extract(opentracing.Binary, &badLimitedReader{R: carrier, N: 4})
	assert.Equal(t, err, opentracing.ErrSpanContextNotFound)
	assert.Nil(t, sc)

	// test reader that returns length header, but then fails after reading another byte
	carrier = new(bytes.Buffer)
	err = binary.Write(carrier, binary.BigEndian, &length) // write length header
	sc, err = tr.Extract(opentracing.Binary, &badLimitedReader{R: carrier, N: 4, errorEOF: true})
	assert.Equal(t, err, opentracing.ErrSpanContextCorrupted)
	assert.Nil(t, sc)

	// test reader that has valid length but bad payload
	carrier = new(bytes.Buffer)
	err = binary.Write(carrier, binary.BigEndian, &length) // write length header
	assert.NoError(t, err)
	_, err = carrier.WriteString("!!!")
	assert.NoError(t, err)
	sc, err = tr.Extract(opentracing.Binary, carrier)
	assert.Equal(t, err, opentracing.ErrSpanContextCorrupted)
	assert.Nil(t, sc)

	// make valid carrier and successfully extract it
	span := tr.StartSpan("op")
	buf := new(bytes.Buffer)
	err = tr.Inject(span.Context(), opentracing.Binary, buf)
	assert.NoError(t, err)
	sc, err = tr.Extract(opentracing.Binary, buf)
	assert.NoError(t, err)
	require.NotNil(t, sc)
	assert.NotEmpty(t, sc.(spanContext).remoteMD)
	assert.Equal(t, sc.(spanContext).remoteMD, span.Context().(spanContext).layer.MetadataString(),
		"extracted context should have same ID as original span")
}

type badMarshaler struct{}

func (*badMarshaler) Marshal(s *tracerState) ([]byte, error)   { return nil, errors.New("marshal error") }
func (*badMarshaler) Unmarshal(b []byte, s *tracerState) error { return errors.New("unmarshal error") }

type badWriter struct{}

func (r badWriter) Write(p []byte) (n int, err error) { return 0, errors.New("write error") }

func TestBinaryInject(t *testing.T) {
	_ = traceview.SetTestReporter()
	tr := NewTracer()
	span := tr.StartSpan("op")

	// inject span to buf successfully
	buf := new(bytes.Buffer)
	err := tr.Inject(span.Context(), opentracing.Binary, buf)
	assert.NoError(t, err)
	assert.NotEmpty(t, buf)

	// inject data to an invalid writer
	buf = new(bytes.Buffer)
	err = tr.Inject(span.Context(), opentracing.Binary, badWriter{})
	assert.Error(t, err)
	assert.Equal(t, 0, buf.Len())

	// inject data that fails to marshal
	buf = new(bytes.Buffer)
	tr.(*Tracer).binaryPropagator.marshaler = &badMarshaler{}
	err = tr.Inject(span.Context(), opentracing.Binary, buf)
	assert.Error(t, err)
	assert.Equal(t, 0, buf.Len())
}

func TestTextMapExtract(t *testing.T) {
	_ = traceview.SetTestReporter()
	tr := NewTracer()
	span := tr.StartSpan("op")
	textCarrier := opentracing.TextMapCarrier{}
	err := span.Tracer().Inject(span.Context(), opentracing.TextMap, textCarrier)
	assert.NoError(t, err)
	assert.NotEmpty(t, textCarrier)

	// extract successfully
	ctx, err := tr.Extract(opentracing.TextMap, textCarrier)
	assert.NotNil(t, ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, ctx.(spanContext).remoteMD)
	assert.Equal(t, ctx.(spanContext).remoteMD, span.Context().(spanContext).layer.MetadataString(),
		"extracted context should have same ID as original span")

	// missing trace ID, true sample flag
	badCarrier := opentracing.TextMapCarrier{}
	badCarrier.Set(fieldNameSampled, "true")
	ctx, err = tr.Extract(opentracing.TextMap, badCarrier)
	assert.Nil(t, ctx)
	assert.Equal(t, opentracing.ErrSpanContextNotFound, err)

	// valid trace ID, false sample flag
	carrier := opentracing.TextMapCarrier{}
	carrier.Set(tv.HTTPHeaderName, textCarrier[tv.HTTPHeaderName])
	carrier.Set(fieldNameSampled, "false")
	ctx, err = tr.Extract(opentracing.TextMap, carrier)
	assert.NotNil(t, ctx)
	assert.NoError(t, err)
	childSpan := tr.StartSpan("op2", opentracing.ChildOf(ctx))
	assert.NotNil(t, childSpan)
	assert.NotNil(t, childSpan.Context().(spanContext).layer)
	assert.False(t, childSpan.Context().(spanContext).layer.IsTracing())

	// valid trace ID, no sampled flag
	tvCarrier := opentracing.TextMapCarrier{}
	tvCarrier.Set(tv.HTTPHeaderName, textCarrier[tv.HTTPHeaderName])
	ctx, err = tr.Extract(opentracing.TextMap, tvCarrier)
	assert.NotNil(t, ctx)
	assert.NoError(t, err)
	assert.True(t, ctx.(spanContext).sampled)
	assert.NotEmpty(t, ctx.(spanContext).remoteMD)
	assert.Equal(t, ctx.(spanContext).remoteMD, span.Context().(spanContext).layer.MetadataString(),
		"extracted context should have same ID as original span")

	// invalid trace ID
	invalidCarrier := opentracing.TextMapCarrier{}
	invalidCarrier.Set(tv.HTTPHeaderName, "!!!")
	ctx, err = tr.Extract(opentracing.TextMap, invalidCarrier)
	assert.Nil(t, ctx)
	assert.Equal(t, opentracing.ErrSpanContextCorrupted, err)

	// invalid sampled flag
	invalidCarrier = opentracing.TextMapCarrier{}
	invalidCarrier.Set(tv.HTTPHeaderName, textCarrier[tv.HTTPHeaderName])
	invalidCarrier.Set(fieldNameSampled, "!!!")
	ctx, err = tr.Extract(opentracing.TextMap, invalidCarrier)
	assert.Nil(t, ctx)
	assert.Equal(t, opentracing.ErrSpanContextCorrupted, err)

}
