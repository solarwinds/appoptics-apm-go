package ao //

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Exporter struct {
	shutdownDelay int // number of seconds to sleep when Shutdown is called, to allow spans to send before short test script exits.
}

const (
	xtraceVersionHeader = "2B"
	sampledFlags        = "01"
	otEventNameKey      = "ot.event_name"
	otStatusCodeKey     = "ot.span_status.code"
	otSpanStatusDescKey = "ot.span_status.description"
)

func fromAttributeValue(attributeValue attribute.Value) interface{} {
	switch attributeValue.Type() {
	case attribute.STRING:
		return attributeValue.AsString()
	case attribute.INT64:
		return attributeValue.AsInt64()
	case attribute.FLOAT64:
		return attributeValue.AsFloat64()
	case attribute.BOOL:
		return attributeValue.AsBool()
	case attribute.STRINGSLICE:
		return attributeValue.AsStringSlice()
	case attribute.INT64SLICE:
		return attributeValue.AsInt64Slice()
	case attribute.FLOAT64SLICE:
		return attributeValue.AsFloat64Slice()
	case attribute.BOOLSLICE:
		return attributeValue.AsBoolSlice()
	default:
		return nil
	}
}

var wsKeyMap = map[string]string{
	"http.method":      "HTTPMethod",
	"http.url":         "URL",
	"http.status_code": "Status",
}
var queryKeyMap = map[string]string{
	"db.connection_string": "RemoteHost",
	"db.name":              "Database",
	"db.statement":         "Query",
	"db.system":            "Flavor",
}

func extractWebserverKvs(span sdktrace.ReadOnlySpan) []interface{} {
	return extractSpecKvs(span, wsKeyMap, "ws")
}

func extractQueryKvs(span sdktrace.ReadOnlySpan) []interface{} {
	return extractSpecKvs(span, queryKeyMap, "query")
}

func extractSpecKvs(span sdktrace.ReadOnlySpan, lookup map[string]string, specValue string) []interface{} {
	attrMap := span.Attributes()
	result := []interface{}{}
	for otKey, aoKey := range lookup {
		for _, attr := range attrMap {
			if string(attr.Key) == otKey {
				result = append(result, aoKey)
				result = append(result, fromAttributeValue(attr.Value))
			}
		}
	}
	if len(result) > 0 {
		result = append(result, "Spec")
		result = append(result, specValue)
	}
	return result
}

func extractKvs(span sdktrace.ReadOnlySpan) []interface{} {
	var kvs []interface{}
	for _, attributeValue := range span.Attributes() {
		if _, ok := wsKeyMap[string(attributeValue.Key)]; ok { // in wsKeyMap, skip it and handle later
			continue
		}
		if _, ok := queryKeyMap[string(attributeValue.Key)]; ok { // in queryKeyMap, skip it and handle later
			continue
		}
		// all other keys
		kvs = append(kvs, string(attributeValue.Key))
		kvs = append(kvs, fromAttributeValue(attributeValue.Value))
	}

	spanStatus := span.Status()
	kvs = append(kvs, otStatusCodeKey)
	kvs = append(kvs, uint32(spanStatus.Code))
	if spanStatus.Code == 1 { // if the span status code is an error, send the description. otel will ignore the description on any other status code
		kvs = append(kvs, otSpanStatusDescKey)
		kvs = append(kvs, spanStatus.Description)
	}
	if !span.Parent().IsValid() { // root span, attempt to extract webserver KVs
		kvs = append(kvs, extractWebserverKvs(span)...)
	}
	kvs = append(kvs, extractQueryKvs(span)...)

	fmt.Println("tracestate FULL:", span.SpanContext().TraceState().String())
	fmt.Println("tracestate key:", VendorID, ", value:", span.SpanContext().TraceState().Get("foo"))
	return kvs
}

func extractInfoEvents(span sdktrace.ReadOnlySpan) [][]interface{} {
	events := span.Events()
	kvs := make([][]interface{}, len(events))

	for i, event := range events {
		kvs[i] = make([]interface{}, 0)
		kvs[i] = append(kvs[i], otEventNameKey)
		kvs[i] = append(kvs[i], string(event.Name))
		for _, attr := range event.Attributes {
			kvs[i] = append(kvs[i], string(attr.Key))
			kvs[i] = append(kvs[i], fromAttributeValue(attr.Value))
		}
	}

	return kvs
}

func getXTraceID(traceID []byte, spanID []byte) string {
	taskId := strings.ToUpper(strings.ReplaceAll(fmt.Sprintf("%0-40v", hex.EncodeToString(traceID)), " ", "0"))
	opId := strings.ToUpper(strings.ReplaceAll(fmt.Sprintf("%0-16v", hex.EncodeToString(spanID)), " ", "0"))
	return xtraceVersionHeader + taskId + opId + sampledFlags
}

func exportSpan(ctx context.Context, s sdktrace.ReadOnlySpan) {
	traceID := s.SpanContext().TraceID()
	spanID := s.SpanContext().SpanID()
	xTraceID := getXTraceID(traceID[:], spanID[:])

	startOverrides := Overrides{
		ExplicitTS:    s.StartTime(),
		ExplicitMdStr: xTraceID,
	}

	endOverrides := Overrides{
		ExplicitTS: s.EndTime(),
	}

	kvs := extractKvs(s)

	infoEvents := extractInfoEvents(s)

	if s.Parent().IsValid() { // this is a child span, not a start of a trace but rather a continuation of an existing one
		parentSpanID := s.Parent().SpanID()
		parentXTraceID := getXTraceID(traceID[:], parentSpanID[:])
		traceContext := FromXTraceIDContext(ctx, parentXTraceID)
		aoSpan, _ := BeginSpanWithOverrides(traceContext, s.Name(), SpanOptions{}, startOverrides)

		// report otel Span Events as AO Info KVs
		for _, infoEventKvs := range infoEvents {
			aoSpan.InfoWithOverrides(Overrides{ExplicitTS: s.StartTime()}, SpanOptions{}, infoEventKvs...)
		}

		aoSpan.EndWithOverrides(endOverrides, kvs...)
	} else { // no parent means this is the beginning of the trace (root span)
		trace := NewTraceWithOverrides(s.Name(), startOverrides, nil)
		trace.SetStartTime(s.StartTime()) //this is for histogram only

		// report otel Span Events as AO Info KVs
		for _, infoEventKvs := range infoEvents {
			trace.InfoWithOverrides(Overrides{ExplicitTS: s.StartTime()}, SpanOptions{}, infoEventKvs...)
		}
		trace.EndWithOverrides(endOverrides, kvs...)
	}
}

func (e *Exporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	WaitForReady(ctx)
	for _, s := range spans {
		exportSpan(ctx, s)
	}
	return nil
}

func (e *Exporter) Shutdown(ctx context.Context) error {
	// Most applications should never set this value, it is only useful for testing short running (cli) scripts.
	if e.shutdownDelay != 0 {
		time.Sleep(time.Duration(e.shutdownDelay) * time.Second)
	}

	Shutdown(ctx)
	return nil
}

// NewExporter creates an instance of the Solarwinds AppOptics exporter for OTEL traces.
func NewExporter(shutdownDelay int) *Exporter {
	return &Exporter{shutdownDelay: shutdownDelay}
}
