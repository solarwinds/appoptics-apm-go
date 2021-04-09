package reporter

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestServerless(t *testing.T) {
	serverless := os.Getenv("APPOPTICS_SERVERLESS")
	defer func() { os.Setenv("APPOPTICS_SERVERLESS", serverless) }()

	os.Setenv("APPOPTICS_SERVERLESS", "true")
	config.Load()

	var sb utils.SafeBuffer
	globalReporter = newServerlessReporter(&sb)
	r := globalReporter.(*serverlessReporter)

	ctx := newTestContext(t)
	ev1, err := ctx.newEvent(LabelInfo, "layer1")
	assert.NoError(t, err)
	assert.NoError(t, r.reportEvent(ctx, ev1))

	ReportSpan(&metrics.HTTPSpanMessage{})
	IncrementMetric("custom_metric", metrics.MetricOptions{Count: 1})

	assert.Nil(t, r.Flush())
	arr := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	evtCnt := 0
	metricCnt := 0
	for _, s := range arr {
		sm := &ServerlessMessage{}
		assert.Nil(t, json.Unmarshal([]byte(s), sm))

		metricCnt += len(sm.Data.Metrics)
		evtCnt += len(sm.Data.Events)

		for _, s := range sm.Data.Events {
			evtByte, err := base64.StdEncoding.DecodeString(s)
			assert.Nil(t, err)
			evt := string(evtByte)
			assert.Contains(t, evt, "layer1")
		}
	}
	assert.Equal(t, 2, metricCnt)
	assert.Equal(t, 1, evtCnt)
}

func TestServerlessShutdown(t *testing.T) {
	serverless := os.Getenv("APPOPTICS_SERVERLESS")
	defer func() { os.Setenv("APPOPTICS_SERVERLESS", serverless) }()

	os.Setenv("APPOPTICS_SERVERLESS", "true")
	config.Load()

	globalReporter = newServerlessReporter(os.Stderr)
	r := globalReporter.(*serverlessReporter)
	assert.Nil(t, r.ShutdownNow())
}
