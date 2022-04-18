package ao

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	ot "go.opentelemetry.io/otel/trace"
)

func setup() (ot.Tracer, func()) {
	tp := trace.NewTracerProvider(
		trace.WithBatcher(NewDummyExporter()),
		trace.WithSampler(NewDummySampler()),
	)
	otel.SetTracerProvider(tp)
	tr := otel.Tracer("foo123", ot.WithInstrumentationVersion("123"), ot.WithSchemaURL("https://www.schema.url/foo123"))

	return tr, func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			fmt.Println(err)
		}
	}
}

type DummySampler struct{}

func (ds *DummySampler) ShouldSample(parameters trace.SamplingParameters) trace.SamplingResult {
	return trace.SamplingResult{
		Decision: trace.RecordAndSample,
	}
}

func (ds *DummySampler) Description() string {
	return "Dummy Sampler"
}

func NewDummySampler() trace.Sampler {
	return &DummySampler{}
}

type DummyExporter struct{}

func NewDummyExporter() *DummyExporter {
	return &DummyExporter{}
}

func (de *DummyExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (de *DummyExporter) Shutdown(ctx context.Context) error {
	return nil
}

func Test_extractKvs_Basic(t *testing.T) {
	tr, teardown := setup()
	defer teardown()
	_, sp := tr.Start(context.Background(), "ROOT SPAN NAME aaaa")
	sp.SetName("span name")

	priority_val := "high"
	priority := attribute.Key("some.priority")
	answers := attribute.Key("some.answers-bool-slice")

	boolSlice := []bool{true, false}

	sp.SetAttributes(priority.String(priority_val), answers.BoolSlice(boolSlice))
	sp.SetStatus(1, "uh oh, problem!") // when the status code is an error, a description "uh oh, problem!" will be inserted into the span as well
	/* Possible otel status codes, see otel/codes.go

	Unset Code = 0

	// Error indicates the operation contains an error.
	Error Code = 1

	// Ok indicates operation has been validated by an Application developers
	// or Operator to have completed successfully, or contain no error.
	Ok Code = 2
	*/

	sp.End()

	kvs := extractKvs(sp.(trace.ReadOnlySpan))

	require.Equal(t, len(kvs), 8, "kvs length didn't match")
	require.Equal(t, kvs[0], string(priority), "kvs[0] didn't match")
	require.Equal(t, kvs[1], priority_val, "kvs[1] didn't match")
	require.Equal(t, kvs[2], string(answers), "kvs[2] didn't match")
	require.Equal(t, kvs[3], boolSlice, "kvs[3] didn't match")
	require.Equal(t, kvs[4], otStatusCodeKey, "kvs[4] didn't match")
	require.Equal(t, kvs[5], uint32(1), "kvs[5] didn't match")
	require.Equal(t, kvs[6], otSpanStatusDescKey, "kvs[6] didn't match") // description will now be present since the status code was 1 == Error
	require.Equal(t, kvs[7], "uh oh, problem!", "kvs[7] didn't match")
}

func Test_extractKvs_WS_Basic(t *testing.T) {
	tr, teardown := setup()
	defer teardown()
	_, sp := tr.Start(context.Background(), "ROOT SPAN NAME aaaa")
	sp.SetName("span name")

	attrs := make([]attribute.KeyValue, len(wsKeyMap))
	for k := range wsKeyMap {
		var attr attribute.KeyValue
		if k == "http.status_code" {
			attr = attribute.KeyValue(attribute.Int(k, 200))
		} else {
			attr = attribute.KeyValue(attribute.String(k, "value for "+k))
		}
		attrs = append(attrs, attr)
	}
	sp.SetAttributes(attrs...)
	sp.End()
	kvs := extractKvs(sp.(trace.ReadOnlySpan))

	require.Equal(t, 10, len(kvs), "kvs length didn't match")
	for idx, v := range kvs {
		if v == "Status" {
			require.Equal(t, kvs[idx+1], int64(200), "status code should be 200")
		}
		if v == "Spec" {
			require.Equal(t, kvs[idx+1], "ws", "spec should be ws")
		}

	}
}

func Test_extractKvs_Empty(t *testing.T) {
	tr, teardown := setup()
	defer teardown()
	_, sp := tr.Start(context.Background(), "ROOT SPAN NAME aaaa")
	sp.SetName("span name")

	attrs := make([]attribute.KeyValue, 0)
	sp.SetAttributes(attrs...)

	kvs := extractKvs(sp.(trace.ReadOnlySpan))

	require.Equal(t, len(kvs), 2, "kvs length didn't match") // its 2 and not 0 because we always add otStatusCodeKey and it's value
	require.Equal(t, kvs[0], otStatusCodeKey, "ot status code key didn't match")
	require.Equal(t, kvs[1], uint32(0), "status code didn't match")
}

func Test_extractInfoEvents(t *testing.T) {
	tr, teardown := setup()
	defer teardown()
	_, sp := tr.Start(context.Background(), "ROOT SPAN NAME aaaa")
	sp.SetName("span name")

	float64attr := attribute.Key("some.float64-slice")
	stringslice := attribute.Key("some.string-slice")
	sp.SetAttributes(float64attr.Float64Slice([]float64{2.3490, 3.14159, 0.49581}), stringslice.StringSlice([]string{"string1", "string2", "string3"}))

	sp.AddEvent("auth", ot.WithAttributes(attribute.String("username", "joe"), attribute.Int("uid", 100)))
	sp.AddEvent("buy", ot.WithAttributes(attribute.String("product", "iPhone"), attribute.Float64("price", 799.99)))
	sp.AddEvent("unsubscribe", ot.WithAttributes(attribute.String("mailing-list-id", "list1"), attribute.Bool("eula-read", true)))

	slInt64 := []int64{-1337, 30, 2, 30000, 45}
	sp.AddEvent("test-int64-slice-event", ot.WithAttributes(attribute.Int64Slice("int64-slice-key", slInt64)))
	slString := []string{"s1", "s2", "s3", "s4"}
	sp.AddEvent("test-string-slice-event", ot.WithAttributes(attribute.StringSlice("string-slice-key", slString)))
	slBool := []bool{true, false, false, true}
	sp.AddEvent("test-bool-slice-event", ot.WithAttributes(attribute.BoolSlice("bool-slice-key", slBool)))
	slFloat64 := []float64{-3.14159, 300.30409, 2, 2.0, 2.001}
	sp.AddEvent("test-float64-slice-event", ot.WithAttributes(attribute.Float64Slice("float64-slice-key", slFloat64)))
	sp.SetStatus(2, "all good!") // description is ignored if the status is not an error
	sp.End()

	kvs := extractKvs(sp.(trace.ReadOnlySpan))
	require.Equal(t, len(kvs), 6, "kvs length mismatch")

	infoEvents := extractInfoEvents(sp.(trace.ReadOnlySpan))
	require.Equal(t, len(infoEvents), 7, "infoEvents length mismatch")

	require.Equal(t, infoEvents[0][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[0][1], "auth", "first event name mismatch")
	require.Equal(t, infoEvents[0][2], "username", "attribute 1 key mismatch")
	require.Equal(t, infoEvents[0][3], "joe", "attribute 1 value mismatch")
	require.Equal(t, infoEvents[0][4], "uid", "attribute 2 key mismatch")
	// even though we set the "uid" value to int 100 above, otel converts int to int64, see: otel/attribute/value/IntValue in value.go
	require.Equal(t, infoEvents[0][5], int64(100), "attribute 2 value mismatch")

	require.Equal(t, infoEvents[1][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[1][1], "buy", "second event name mismatch")
	require.Equal(t, infoEvents[1][2], "product", "second event, attribute 1 key mismatch")
	require.Equal(t, infoEvents[1][3], "iPhone", "second event, attribute 1 value mismatch")
	require.Equal(t, infoEvents[1][4], "price", "second event, attribute 2 key mismatch")
	require.Equal(t, infoEvents[1][5], float64(799.99), "second event, attribute 2 value mismatch")

	require.Equal(t, infoEvents[2][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[2][1], "unsubscribe", "third event name mismatch")
	require.Equal(t, infoEvents[2][2], "mailing-list-id", "third event, attribute 1 key mismatch")
	require.Equal(t, infoEvents[2][3], "list1", "third event, attribute 1 value mismatch")
	require.Equal(t, infoEvents[2][4], "eula-read", "third event, attribute 2 key mismatch")
	require.Equal(t, infoEvents[2][5], true, "third event, attribute 2 value mismatch")

	// Test slice data types
	require.Equal(t, infoEvents[3][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[3][1], "test-int64-slice-event", "int64 slice event name mismatch")
	require.Equal(t, infoEvents[3][2], "int64-slice-key", "int64 slice key mismatch")
	require.Equal(t, infoEvents[3][3], slInt64, "int64 slice values mismatch")

	require.Equal(t, infoEvents[4][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[4][1], "test-string-slice-event", "string slice event name mismatch")
	require.Equal(t, infoEvents[4][2], "string-slice-key", "string slice key mismatch")
	require.Equal(t, infoEvents[4][3], slString, "string slice values mismatch")

	require.Equal(t, infoEvents[5][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[5][1], "test-bool-slice-event", "bool slice event name mismatch")
	require.Equal(t, infoEvents[5][2], "bool-slice-key", "bool slice key mismatch")
	require.Equal(t, infoEvents[5][3], slBool, "bool slice values mismatch")

	require.Equal(t, infoEvents[6][0], otEventNameKey, "ot event key mismatch")
	require.Equal(t, infoEvents[6][1], "test-float64-slice-event", "float64 slice event name mismatch")
	require.Equal(t, infoEvents[6][2], "float64-slice-key", "float64 slice key mismatch")
	require.Equal(t, infoEvents[6][3], slFloat64, "float64 slice values mismatch")

	// fmt.Println("infoEvents")
	// for idx, v := range infoEvents {
	// 	fmt.Println("idx:", idx)
	// 	for k2, v2 := range v {
	// 		fmt.Println("\tidx:", k2, " v:", v2)
	// 	}
	// }
}

func Test_extractKvs_WS_NotRootSpan(t *testing.T) {
	tr, teardown := setup()
	defer teardown()
	ctx, sp := tr.Start(context.Background(), "ROOT SPAN NAME aaaa")
	sp.SetName("span name")
	sp.End()

	kvs := extractKvs(sp.(trace.ReadOnlySpan))

	require.Equal(t, len(kvs), 2, "kvs length didn't match")

	// Check to make sure that we do not read webserver keys from child spans, only from root span!
	_, sp2 := tr.Start(ctx, "child span")
	attrs := make([]attribute.KeyValue, len(wsKeyMap))
	for k := range wsKeyMap {
		var attr attribute.KeyValue
		if k == "http.status_code" {
			attr = attribute.KeyValue(attribute.Int(k, 200))
		} else {
			attr = attribute.KeyValue(attribute.String(k, "value for "+k))
		}
		attrs = append(attrs, attr)
	}
	sp2.SetAttributes(attrs...)

	kvs = extractKvs(sp2.(trace.ReadOnlySpan))

	require.Equal(t, len(kvs), 2, "kvs length didn't match")
	for idx, v := range kvs {
		if v == "Status" {
			require.Equal(t, kvs[idx+1], int64(200), "status code should be 200")
			break
		}
	}
}

func Test_extractKvs_Query_Basic(t *testing.T) {
	tr, teardown := setup()
	defer teardown()
	_, sp := tr.Start(context.Background(), "ROOT SPAN NAME aaaa")
	sp.SetName("span name")

	// "db.connection_string": "RemoteHost",
	// "db.name":              "Database",
	// "db.statement":         "Query",
	// "db.system":            "Flavor",

	attrs := make([]attribute.KeyValue, len(queryKeyMap))
	for k := range queryKeyMap {
		attr := attribute.KeyValue(attribute.String(k, "value for "+k))
		attrs = append(attrs, attr)
	}
	sp.SetAttributes(attrs...)
	sp.End()
	kvs := extractKvs(sp.(trace.ReadOnlySpan))
	require.Equal(t, 12, len(kvs), "kvs length didn't match")

	found := 0
	// ensure all queryKeyMap keys are found in the kvs produced since we populated them all above when creating the span attributes
	for _, queryKey := range queryKeyMap {
		for _, v := range kvs {
			if queryKey == v {
				found++
			}
		}
	}
	require.Equal(t, len(queryKeyMap), found, "should find all queryKeyMap keys in extracted kvs")

	// ensure we have the Spec key that's set to query
	for idx, v := range kvs {
		if v == "Spec" {
			require.Equal(t, "query", kvs[idx+1], "spec should be query")
			break
		}
	}
}

func Test_getXTraceID(t *testing.T) {
	var traceID ot.TraceID
	var spanID ot.SpanID

	traceID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID = [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	xTraceID := getXTraceID(traceID[:], spanID[:])
	//2B 0102030405060708090A0B0C0D0E0F10 00000000 0102030405060708 01
	expectedXTraceID := strings.ToUpper(xtraceVersionHeader + hex.EncodeToString(traceID[:]) + "00000000" + hex.EncodeToString(spanID[:]) + sampledFlags)
	require.Equal(t, expectedXTraceID, xTraceID, "xTraceID should be equal")
}
