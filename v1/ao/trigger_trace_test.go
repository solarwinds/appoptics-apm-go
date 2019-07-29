package ao_test

import (
	"testing"

	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/stretchr/testify/assert"
)

func TestTriggerTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger_trace;pd_keys=lo:se,check-id:123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger_trace=ok", rHeader.Get("X-Trace-Options-Response"))
	assert.NotEmpty(t, rHeader.Get("X-Trace"))
}
