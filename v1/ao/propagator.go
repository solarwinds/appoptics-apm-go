package ao

import (
	"context"
	"fmt"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceState  = "tracestate"
	traceParent = "traceparent"
	VendorID    = "sw"
)

type SolarwindsPropagator struct{}

var _ propagation.TextMapPropagator = SolarwindsPropagator{}

func (swp SolarwindsPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	fmt.Println("SolarwindsPropagator Inject called")
	sc := trace.SpanContextFromContext(ctx)

	if !sc.IsValid() {
		fmt.Println("Inject: invalid span context, exiting.")
		return
	}
	ts := sc.TraceState()

	fmt.Println("ts.String()   TRACESTATE BEFORE INSERT OF OUR VALUE:" + ts.String())
	fmt.Println("carrier.Get() TRACESTATE IN CARRIER BEFORE OUR VALUE: " + carrier.Get(traceState))

	ts, err := trace.ParseTraceState(carrier.Get(traceState))
	if err != nil {
		fmt.Println("err parsing trace state from string!!!")
		return
	}
	fmt.Println("ts.String()   TRACESTATE BEFORE INSERT OF OUR VALUE, AFTER PARSED FROM CARRIER:" + ts.String())

	spanID := sc.SpanID().String()
	sampleFlag := "00"
	if sc.TraceFlags().IsSampled() {
		sampleFlag = "01"
	}
	swData := spanID + "-" + sampleFlag

	ts, err = ts.Insert(VendorID, swData)
	if err != nil {
		fmt.Println("... ERROR ERROR ERROR")
		fmt.Println(err)
		fmt.Println("... ERROR ERROR ERROR")
		return
	}

	fmt.Println("TRACESTATE AFTER INSERT OF OUR VALUE:" + ts.String())
	carrier.Set(traceState, ts.String())

	// ts := carrier.Get(traceState)
	// fmt.Println("SolarwindsPropagator(Inject) from carrier.Get, ts=", ts)
	// fmt.Println(">>> If we get it from sc though, ts=", sc.TraceState().String())
	// if ts == "" { // if there is no trace state, create one w/ our vendor id
	// 	pair := swp.traceStateCreate(sc)
	// 	fmt.Println("traceStateCreate returns pair:", pair)
	// 	carrier.Set(traceState, pair[0]+"="+pair[1])
	// 	return
	// }

	// // if tracestate exists update it to either add our vendor id or update the value already stored there
	// ts = swp.traceStateUpdate(sc, ts)
	// fmt.Println("ts after update:", ts)
	// carrier.Set(traceState, ts)
}

func (swp SolarwindsPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	fmt.Println("SolarwindsPropagator(Extract) called")
	sc := trace.SpanContextFromContext(ctx)

	fmt.Println("TRACESTATE IN sc.TraceState().String():" + sc.TraceState().String())
	fmt.Println("traceState in carrier is:")
	fmt.Println("---- [begin] ----")
	fmt.Println(carrier.Get(traceState))
	fmt.Println("---- [end] ----")
	fmt.Println("CARRIER KEYS: ", carrier.Keys())
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	ctx = context.WithValue(ctx, "X-Trace-Options", carrier.Get("X-Trace-Options"))
	ctx = context.WithValue(ctx, "X-Trace-Options-Signature", carrier.Get("X-Trace-Options-Signature"))

	return ctx
}

// Fields returns the keys who's values are set with Inject.
func (swp SolarwindsPropagator) Fields() []string {
	return []string{traceState}
}

// traceStateCreate takes a trace.SpanContext and returns a brand new tracestate string with our VendorID,
// ie: sw=SpanID-SampledFlag
// func (swp SolarwindsPropagator) traceStateCreate(sc trace.SpanContext) []string {
// 	fmt.Println("SolarwindsPropagator(traceStateCreate) called")
// 	spanID := sc.SpanID().String()
// 	sampleFlag := "00"
// 	if sc.TraceFlags().IsSampled() {
// 		sampleFlag = "01"
// 	}
// 	traceState := spanID + "-" + sampleFlag
// 	fmt.Println("traceStateCreate, returning:", traceState)
// 	return []string{VendorID, traceState}
// }

// // traceStateUpdate takes a trace.SpanContext and returns an updated tracestate string, prepending our VendorID and our values.
// // If we already are part of the trace state then we need to update the existing value.
// // ie: sw=SpanID-SampledFlag,vendor2=foo,vendor3=bar,etc...
// func (swp SolarwindsPropagator) traceStateUpdate(sc trace.SpanContext, ts string) string {
// 	orderedTraceState := make([]string, 0)
// 	ourVendorIDFound := false
// 	fmt.Println("SolarwindsPropagator(traceStateUpdate) called")
// 	fmt.Println("ts=", ts)
// 	for _, vendorData := range strings.Split(ts, ",") {
// 		fmt.Println("vendorData=", vendorData)
// 		s := strings.Split(vendorData, "=")
// 		if len(s) != 2 {
// 			fmt.Println("bad vendor data")
// 			continue
// 		}
// 		vendorId := s[0]
// 		vendorData := s[1]

// 		if vendorId == VendorID { // this is our vendor, let's update the value
// 			fmt.Println("found our vendor data", vendorId, vendorData)
// 			orderedTraceState = append(orderedTraceState, []string{vendorId, "ournewdata"}...) // FIXME: placeholder
// 			ourVendorIDFound = true
// 		} else { // some other vendor's data... we do not care
// 			orderedTraceState = append(orderedTraceState, []string{vendorId, vendorData}...)
// 			fmt.Println("some other vendor data found", vendorId, vendorData)
// 		}
// 	}
// 	// if we still don't have our VendorID in the tracestate, lets create add it
// 	if !ourVendorIDFound {
// 		orderedTraceState = append(swp.traceStateCreate(sc), orderedTraceState...)
// 	}

// 	fmt.Println("---------- orderedTraceState begin -----------")
// 	fmt.Println(orderedTraceState)
// 	fmt.Println("---------- orderedTraceState end -----------")
// 	// sw, foo
// 	// dummy1, injected
// 	newTraceState := ""
// 	for i := 0; i <= len(orderedTraceState)-1; i += 2 {
// 		if len(newTraceState) > 0 {
// 			newTraceState += ","
// 		}
// 		newTraceState += orderedTraceState[i] + "=" + orderedTraceState[i+1]
// 	}
// 	return newTraceState
// }

type SWSampler struct{}

var _ sdktrace.Sampler = SWSampler{}

func (ds SWSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	fmt.Println("in ShouldSample of SWSampler")

	fmt.Println("parameters.Attributes:")
	fmt.Println("[start]---------------------")
	fmt.Println(parameters.Attributes)
	fmt.Println("[end  ]---------------------")

	traced := false
	parentContext := parameters.ParentContext
	fmt.Println("X-Trace-Options in parentContext?: ", parentContext.Value("X-Trace-Options"))
	if parentContext != nil {
		fmt.Println("ParentContext is not nil!")
		sc := trace.SpanFromContext(parentContext)
		traced = sc.IsRecording()
		fmt.Println("sc.IsRecording", sc.IsRecording())

		fmt.Println("sc.SpanContext().MarshalJSON()")
		fmt.Println("[start]---------------------")
		s, err := sc.SpanContext().MarshalJSON()
		if err != nil {
			return sdktrace.SamplingResult{
				Decision: sdktrace.RecordAndSample,
			}
		}
		fmt.Println(string(s))
		fmt.Println("[end  ]---------------------")

	} else {
		fmt.Println("ParentContext is nil...")
	}

	url := "" // FIXME: how to decide and set this value?

	ttMode := reporter.ParseTriggerTrace(parentContext.Value("X-Trace-Options").(string), parentContext.Value("X-Trace-Options-Signature").(string))

	traceDecision, xTraceOptionsResponse := reporter.ShouldTraceRequestWithURL(parameters.Name, traced, url, ttMode)

	fmt.Printf("ShouldSample name(%v) -> Decision(%v), xTraceOptsResponse(%v)\n", parameters.Name, traceDecision, xTraceOptionsResponse)

	// XXX: do we need any of these to be returned from ShouldTraceRequestWithURL?
	// XXX: do we care about url at all here?

	// type SampleDecision struct {
	// 	trace  bool
	// 	rate   int
	// 	source sampleSource
	// 	// if the request is disabled from tracing in a per-transaction level or for
	// 	// the entire service.
	// 	enabled       bool
	// 	xTraceOptsRsp string
	// 	bucketCap     float64
	// 	bucketRate    float64
	// }

	if traceDecision {
		return sdktrace.SamplingResult{
			Decision: sdktrace.RecordAndSample,
		}
		// TODO: return trace state
	}

	return sdktrace.SamplingResult{
		Decision: sdktrace.Drop,
		// TODO: return trace state
	}
}

func (ds SWSampler) Description() string {
	return "SolarWinds Sampler"
}

func NewSampler() sdktrace.Sampler {
	return SWSampler{}
}
