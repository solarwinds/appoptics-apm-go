package reporter

import (
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/bson"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/host"
)

// EncodedEvent denotes the event after encoding.
type EncodedEvent struct {
	*bson.Buffer
}

// NewEncodedEvent creates a new EncodedEvent object
func NewEncodedEvent(layer, label, xTrace, edge string, ts int64) EncodedEvent {
	ee := EncodedEvent{bson.NewBuffer()}
	ee.AppendString("_V", "1")
	ee.AppendString("X-Trace", xTrace)
	ee.AppendString("Label", label)
	ee.AppendString("Layer", layer)
	if edge != "" {
		ee.AppendString(EdgeKey, edge)
	}
	ee.AppendInt64("Timestamp_u", ts)
	ee.AppendString("Hostname", host.Hostname())
	ee.AppendInt("PID", host.PID())

	return ee
}

// Report report itself via the default global reporter
func (ee EncodedEvent) Report() {
	ee.Finish()
	ee.reportWith(globalReporter)
}

func (ee EncodedEvent) reportWith(r reporter) {
	r.reportBuf(ee.GetBuf())
}

// XTraceIDFrom get a new X-Trace ID from the existing one, with the op Id
// randomized
func XTraceIDFrom(mdStr string) string {
	md := &oboeMetadata{}
	md.FromString(mdStr)
	md.SetRandom()
	xTrace, err := md.ToString()
	if err != nil {
		return ""
	} else {
		return xTrace
	}
}
