package trace

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"

	"go.opentelemetry.io/otel/api/core"
	ot "go.opentelemetry.io/otel/sdk/export/trace"
)

// Exporter exports the OpenTelemetry trace data to the AppOptics backend. It's
// an implementation of the OpenTelemetry's trace.SpanSyncer interface.
type Exporter struct{}

// Options are the options for initializing the AppOptics OpenTelemetry exporter.
type Options struct {
	// TODO
}

// NewExporter creates and returns a new Exporter instance
func NewExporter(o Options) (*Exporter, error) {
	return &Exporter{}, nil
}

// ExportSpan converts the data of the OpenTelemetry trace and send it to the
// AppOptics backend.
func (e *Exporter) ExportSpan(ctx context.Context, data *ot.SpanData) {
	if data == nil {
		return
	}

	spanName := fmt.Sprintf("%s.%s", data.SpanKind, data.Name)
	ts := data.StartTime.UnixNano() / 1000
	xTrace := e.getXTraceID(data.SpanContext.SpanID, data.SpanContext.TraceID, data.SpanContext.IsSampled())
	edge := e.getEdge(data.ParentSpanID)

	entry := reporter.NewEncodedEvent(spanName, "entry", xTrace, edge, ts)
	e.addAttributes(entry, data.Attributes)
	if !data.ParentSpanID.IsValid() {
		entry.AppendString("TransactionName", spanName)
	}
	entry.Report()

	infoEdge := e.getEdge(data.SpanContext.SpanID)
	for _, ev := range data.MessageEvents {
		xid := reporter.NewXTraceIDFrom(xTrace)
		info := reporter.NewEncodedEvent(spanName, "info",
			xid, infoEdge, ev.Time.UnixNano()/1000)
		info.AppendString("Name", ev.Name)
		e.addAttributes(info, ev.Attributes)
		info.Report()

		infoEdge = reporter.GetOpIDFromXTraceID(xid)
	}

	exitXTrace := reporter.NewXTraceIDFrom(xTrace)
	exitTs := data.EndTime.UnixNano() / 1000
	exitEdge := infoEdge
	exit := reporter.NewEncodedEvent(spanName, "exit", exitXTrace, exitEdge, exitTs)
	exit.Report()
}

func (e *Exporter) addAttributes(ee reporter.EncodedEvent, attrs []core.KeyValue) {
	for _, attr := range attrs {
		key := string(attr.Key)
		val := attr.Value
		switch val.Type() {
		case core.INVALID:
			// no-op
		case core.BOOL:
			ee.AppendBool(key, val.AsBool())
		case core.INT32:
			ee.AppendInt32(key, val.AsInt32())
		case core.INT64, core.UINT32, core.UINT64:
			ee.AppendInt64(key, val.AsInt64())
		case core.FLOAT32, core.FLOAT64:
			ee.AppendFloat64(key, float64(val.AsFloat32()))
		case core.STRING:
			ee.AppendString(key, val.AsString())
		}
	}
}

func (e *Exporter) getEdge(spanID core.SpanID) string {
	if !spanID.IsValid() {
		return ""
	}

	buf := bytes.Buffer{}

	spanIDStr := spanID.String()
	buf.WriteString(spanIDStr)
	buf.WriteString(strings.Repeat("0", reporter.OboeMaxOpIDLen*2-len(spanIDStr)))

	return strings.ToUpper(buf.String())
}

func (e *Exporter) getXTraceID(spanID core.SpanID, traceID core.TraceID, sampled bool) string {
	buf := bytes.Buffer{}
	buf.WriteString("2B")

	traceIDStr := traceID.String()
	buf.WriteString(traceIDStr)
	buf.WriteString(strings.Repeat("0", reporter.OboeMaxTaskIDLen*2-len(traceIDStr)))
	spanIDStr := spanID.String()
	buf.WriteString(spanIDStr)
	buf.WriteString(strings.Repeat("0", reporter.OboeMaxOpIDLen*2-len(spanIDStr)))
	if sampled {
		buf.WriteString("01")
	} else {
		buf.WriteString("00")
	}
	return strings.ToUpper(buf.String())
}
