package http_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/stretchr/testify/assert"
)

// trigger trace enabled, unsigned TT request with duplicate custom key: custom-key1
func TestTriggerTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123;custom-key1=hello;custom-key2=world;custom-key1=hi",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, true, n.Map["TriggeredTrace"])
			assert.Equal(t, "hello", n.Map["custom-key1"])
			assert.Equal(t, "world", n.Map["custom-key2"])
			assert.Equal(t, "lo:se,check-id:123", n.Map["PDKeys"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ok", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// trigger trace enabled, no available settings, unsigned TT request
func TestUnsignedTriggerTraceNoSetting(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.NoSettingST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=settings-not-available", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// trigger trace enabled, no available settings, signed TT request
func TestSignedTriggerTraceNoSetting(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.NoSettingST))
	ts := time.Now().Unix()
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=settings-not-available;auth=no-signature-key", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// only trigger trace enabled, TT request with custom key/value surrounded by spaces
func TestTriggerTraceWithCustomKey(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.TriggerTraceOnlyST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace; custom-key1 = \tvalue1 ",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, "value1", n.Map["custom-key1"])
			assert.Equal(t, true, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ok", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// limited trigger trace token bucket, unsigned TT requests
func TestTriggerTraceRateLimited(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.LimitedTriggerTraceST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;custom-key1=value1",
	}

	var rrs []*httptest.ResponseRecorder
	numRequests := 5
	for i := 0; i < numRequests; i++ {
		rrs = append(rrs, httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd))
	}

	r.Close(0) // Don't check number of events here

	numEvts := len(r.EventBufs)
	assert.True(t, numEvts < 10)

	limited := 0
	triggerTraced := 0

	for _, rr := range rrs {
		rsp := rr.Header().Get("X-Trace-Options-Response")
		if rsp == "trigger-trace=ok" {
			triggerTraced++
		} else if rsp == "trigger-trace=rate-exceeded" {
			limited++
		}
	}
	assert.True(t, (limited+triggerTraced) == numRequests)
	assert.True(t, triggerTraced*2 == numEvts,
		fmt.Sprintf("triggerTraced=%d, numEvts=%d", triggerTraced, numEvts))
}

// limited trigger trace token bucket, signed TT requests
func TestRelaxedTriggerTraceRateLimited(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.LimitedTriggerTraceST))
	ts := time.Now().Unix()
	opts := fmt.Sprintf("trigger-trace;custom-key1=value1;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
	}

	var rrs []*httptest.ResponseRecorder
	numRequests := 5
	for i := 0; i < numRequests; i++ {
		rrs = append(rrs, httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd))
	}

	r.Close(0) // Don't check number of events here

	numEvts := len(r.EventBufs)
	assert.True(t, numEvts < 10)

	limited := 0
	triggerTraced := 0

	for _, rr := range rrs {
		rsp := rr.Header().Get("X-Trace-Options-Response")
		if rsp == "trigger-trace=ok;auth=ok" {
			triggerTraced++
		} else if rsp == "trigger-trace=rate-exceeded;auth=ok" {
			limited++
		}
	}
	assert.True(t, (limited+triggerTraced) == numRequests)
	assert.True(t, triggerTraced*2 == numEvts,
		fmt.Sprintf("triggerTraced=%d, numEvts=%d", triggerTraced, numEvts))
}

// trigger trace: local disabled, remote enabled, unsigned TT request
func TestTriggerTraceLocalDisabledRemoteEnabled(t *testing.T) {
	_ = os.Setenv("APPOPTICS_TRIGGER_TRACE", "false")
	_ = config.Load()

	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=trigger-tracing-disabled", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))

	_ = os.Unsetenv("APPOPTICS_TRIGGER_TRACE")
	_ = config.Load()
}

// trigger trace: local enabled, remote disabled, unsigned TT request
func TestTriggerTraceLocalEnabledRemoteDisabled(t *testing.T) {
	_ = os.Setenv("APPOPTICS_TRIGGER_TRACE", "true")
	_ = config.Load()

	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.NoTriggerTraceST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=trigger-tracing-disabled", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))

	_ = os.Unsetenv("APPOPTICS_TRIGGER_TRACE")
	_ = config.Load()
}

// trigger trace enabled but tracing mode disabled locally
func TestTriggerTraceEnabledTracingModeDisabled(t *testing.T) {
	_ = os.Setenv("APPOPTICS_TRACING_MODE", "disabled")
	_ = config.Load()

	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=tracing-disabled", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))

	_ = os.Unsetenv("APPOPTICS_TRACING_MODE")
	_ = config.Load()
}

// trigger trace with service/URL based trace filtering
func TestTriggerTraceWithURLFiltering(t *testing.T) {
	reporter.ReloadURLsConfig([]config.TransactionFilter{
		{"url", `hello`, nil, "disabled"}, // trace is disabled for this URL pattern
	})

	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=tracing-disabled", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))

	reporter.ReloadURLsConfig(nil)
}

// no trigger trace enabled, unsigned TT request, invalid key (contains spaces)
func TestNoTriggerTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "pd keys=lo:se,check-id:123;custom-key1=value1",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, "value1", n.Map["custom-key1"])
			assert.Nil(t, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=not-requested;ignored=pd keys", rHeader.Get("X-Trace-Options-Response"))
	assert.NotEmpty(t, rHeader.Get("X-Trace"))
}

// trigger trace enabled, invalid trigger trace flag in TT request
func TestNoTriggerTraceInvalidFlag(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace=1;tigger_trace",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Nil(t, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=not-requested;ignored=tigger_trace,trigger-trace", rHeader.Get("X-Trace-Options-Response"))
	assert.NotEmpty(t, rHeader.Get("X-Trace"))
}

// trigger trace enabled, unsigned TT request, invalid custom keys
func TestTriggerTraceInvalidFlag(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;foo=bar;app_id=123",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, true, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ok;ignored=foo,app_id", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// trigger trace enabled, unsigned TT request with not-traced X-Trace ID (obey the X-Trace ID)
func TestTriggerTraceWithNotTracedXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;not-valid-opt=value2",
		"X-Trace":         "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF00",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ignored;ignored=not-valid-opt", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// trigger trace enabled, non-TT request, not-traced X-Trace ID
func TestNoTriggerTraceWithNotTracedXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "custom-key1=value1;not-valid-opt=value2",
		"X-Trace":         "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF00",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=not-requested;ignored=not-valid-opt", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// non-TT request with traced X-Trace ID
func TestNoTriggerTraceWithXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "pd-keys=lo:se,check-id:123;custom-key1=value1;not-valid-opt=value2",
		"X-Trace":         "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF01",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{{"Edge", "2C5EFEA7749039AF"}}, Callback: func(n g.Node) {
			assert.Equal(t, "value1", n.Map["custom-key1"])
			assert.Equal(t, "lo:se,check-id:123", n.Map["PDKeys"])
			assert.Nil(t, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=not-requested;ignored=not-valid-opt", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// TT request with traced X-Trace ID
func TestTriggerTraceWithXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123;custom-key1=value1;not-valid-opt=value2",
		"X-Trace":         "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF01",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{{"Edge", "2C5EFEA7749039AF"}}, Callback: func(n g.Node) {
			assert.Equal(t, "value1", n.Map["custom-key1"])
			assert.Equal(t, "lo:se,check-id:123", n.Map["PDKeys"])
			assert.Nil(t, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ignored;ignored=not-valid-opt", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// TT request with invalid X-Trace ID (obey trigger trace flag)
func TestTriggerTraceInvalidXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options": "trigger-trace;pd-keys=lo:se,check-id:123",
		"X-Trace":         "invalid-value",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, true, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ok", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// signed TT request
func TestRelaxedTriggerTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.RelaxedTriggerTraceOnlyST))
	ts := time.Now().Unix()
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, true, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ok;auth=ok", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// signed TT request with invalid timestamp
func TestRelaxedTriggerTraceTSNotInScope(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	ts := time.Now().Unix() - 60*6
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "auth=bad-timestamp", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// signed TT request with invalid signature
func TestRelaxedTriggerTraceInvalidSignature(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	hd := map[string]string{
		"X-Trace-Options":           fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;ts=%d", time.Now().Unix()),
		"X-Trace-Options-Signature": "invalidSignature",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "auth=bad-signature", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// signed TT request with traced X-Trace
func TestRelaxedTriggerTraceWithTracedXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	ts := time.Now().Unix()
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;custom-key1=value1;not-valid-opt=value2;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
		"X-Trace":                   "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF01",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{{"Edge", "2C5EFEA7749039AF"}}, Callback: func(n g.Node) {
			assert.Nil(t, n.Map["TriggeredTrace"])
			assert.Equal(t, "value1", n.Map["custom-key1"])
			assert.Equal(t, "lo:se,check-id:123", n.Map["PDKeys"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ignored;auth=ok;ignored=not-valid-opt", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// signed TT request with not-traced X-Trace
func TestRelaxedTriggerTraceWithNotTracedXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	ts := time.Now().Unix()
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;custom-key1=value1;not-valid-opt=value2;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
		"X-Trace":                   "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF00",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ignored;auth=ok;ignored=not-valid-opt", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}

// signed TT request with bad timestamp and traced X-Trace
func TestRelaxedTriggerTraceWithBadTsAndTracedXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	ts := time.Now().Unix() - 360
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;custom-key1=value1;not-valid-opt=value2;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
		"X-Trace":                   "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF01",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{{"Edge", "2C5EFEA7749039AF"}}, Callback: func(n g.Node) {
			assert.Nil(t, n.Map["TriggeredTrace"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
		}},
	})
	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ignored;auth=bad-timestamp", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "01"))
}

// signed TT request with bad timestamp and not-traced X-Trace
func TestRelaxedTriggerTraceWithBadTsAndNotTracedXTrace(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterSettingType(reporter.DefaultST))
	ts := time.Now().Unix() - 360
	opts := fmt.Sprintf("trigger-trace;pd-keys=lo:se,check-id:123;custom-key1=value1;not-valid-opt=value2;ts=%d", ts)
	hd := map[string]string{
		"X-Trace-Options":           opts,
		"X-Trace-Options-Signature": reporter.HmacHash([]byte(reporter.TestToken), []byte(opts)),
		"X-Trace":                   "2B987445277543FF9C151D0CDE6D29B6E21603D5DB2C5EFEA7749039AF00",
	}

	rr := httpTestWithEndpointWithHeaders(handler200, "http://test.com/hello", hd)
	r.Close(0)

	rHeader := rr.Header()
	assert.EqualValues(t, "trigger-trace=ignored;auth=bad-timestamp", rHeader.Get("X-Trace-Options-Response"))
	assert.True(t, strings.HasSuffix(rHeader.Get("X-Trace"), "00"))
}
