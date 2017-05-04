package tracelytics

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	ot "github.com/opentracing/opentracing-go"
	"github.com/tracelytics/go-traceview/v1/tv"
)

const (
	prefixBaggage    = "ot-baggage-"
	fieldNameSampled = "ot-tracer-sampled"
)

// Inject belongs to the Tracer interface.
func (t *Tracer) Inject(sc ot.SpanContext, format interface{}, carrier interface{}) error {
	switch format {
	case ot.TextMap, ot.HTTPHeaders:
		return t.textMapPropagator.Inject(sc, carrier)
	case ot.Binary:
		return t.binaryPropagator.Inject(sc, carrier)
	}
	return ot.ErrUnsupportedFormat
}

// Extract belongs to the Tracer interface.
func (t *Tracer) Extract(format interface{}, carrier interface{}) (ot.SpanContext, error) {
	switch format {
	case ot.TextMap, ot.HTTPHeaders:
		return t.textMapPropagator.Extract(carrier)
	case ot.Binary:
		return t.binaryPropagator.Extract(carrier)
	}
	return nil, ot.ErrUnsupportedFormat
}

type textMapPropagator struct{}
type binaryPropagator struct{}

func (p *textMapPropagator) Inject(spanCtx ot.SpanContext, opaqueCarrier interface{}) error {
	sc, ok := spanCtx.(spanContext)
	if !ok {
		return ot.ErrInvalidSpanContext
	}
	carrier, ok := opaqueCarrier.(ot.TextMapWriter)
	if !ok {
		return ot.ErrInvalidCarrier
	}
	if md := sc.Layer.MetadataString(); md != "" {
		carrier.Set(tv.HTTPHeaderName, md)
	}
	carrier.Set(fieldNameSampled, strconv.FormatBool(sc.Layer.IsTracing()))

	for k, v := range sc.Baggage {
		carrier.Set(prefixBaggage+k, v)
	}
	return nil
}

type tracerState struct {
	XTraceID     string            `json:"xtrace_id,omitempty"`
	Sampled      bool              `json:"sampled,omitempty"`
	BaggageItems map[string]string `json:"baggage_items,omitempty"`
}

func (p *binaryPropagator) Inject(spanCtx ot.SpanContext, opaqueCarrier interface{}) error {
	sc, ok := spanCtx.(spanContext)
	if !ok {
		return ot.ErrInvalidSpanContext
	}
	carrier, ok := opaqueCarrier.(io.Writer)
	if !ok {
		return ot.ErrInvalidCarrier
	}

	state := tracerState{
		XTraceID:     sc.Layer.MetadataString(),
		Sampled:      sc.Layer.IsTracing(),
		BaggageItems: sc.Baggage,
	}

	b, err := json.Marshal(&state)
	if err != nil {
		return err
	}

	// Write the length of the marshalled binary to the writer.
	length := uint32(len(b))
	if err := binary.Write(carrier, binary.BigEndian, &length); err != nil {
		return err
	}

	_, err = carrier.Write(b)
	return err
}

func (p *binaryPropagator) Extract(opaqueCarrier interface{}) (ot.SpanContext, error) {
	carrier, ok := opaqueCarrier.(io.Reader)
	if !ok {
		return nil, ot.ErrInvalidCarrier
	}

	// Read the length of the marshalled binary
	var length uint32
	if err := binary.Read(carrier, binary.BigEndian, &length); err != nil {
		return nil, ot.ErrSpanContextCorrupted
	}
	buf := make([]byte, length)
	if n, err := carrier.Read(buf); err != nil {
		if n > 0 {
			return nil, ot.ErrSpanContextCorrupted
		}
		return nil, ot.ErrSpanContextNotFound
	}

	ctx := tracerState{}
	if err := json.Unmarshal(buf, &ctx); err != nil {
		return nil, ot.ErrSpanContextCorrupted
	}

	return spanContext{
		remoteMD: ctx.XTraceID,
		sampled:  ctx.Sampled,
		Baggage:  ctx.BaggageItems,
	}, nil
}

func (p *textMapPropagator) Extract(opaqueCarrier interface{}) (ot.SpanContext, error) {
	carrier, ok := opaqueCarrier.(ot.TextMapReader)
	if !ok {
		return nil, ot.ErrInvalidCarrier
	}
	requiredFieldCount := 0
	var xTraceID string
	var sampled bool
	var err error
	decodedBaggage := make(map[string]string)
	err = carrier.ForeachKey(func(k, v string) error {
		switch strings.ToLower(k) {
		case strings.ToLower(tv.HTTPHeaderName):
			xTraceID = v // XXX could validate X-Trace metadata here
			err = nil
			if err != nil {
				return ot.ErrSpanContextCorrupted
			}
			requiredFieldCount++
		case fieldNameSampled:
			sampled, err = strconv.ParseBool(v)
			if err != nil {
				return ot.ErrSpanContextCorrupted
			}
			requiredFieldCount++
		default:
			lowercaseK := strings.ToLower(k)
			if strings.HasPrefix(lowercaseK, prefixBaggage) {
				decodedBaggage[strings.TrimPrefix(lowercaseK, prefixBaggage)] = v
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if requiredFieldCount == 0 {
		return nil, ot.ErrSpanContextNotFound
	}

	return spanContext{
		remoteMD: xTraceID,
		sampled:  sampled,
		Baggage:  decodedBaggage,
	}, nil
}
