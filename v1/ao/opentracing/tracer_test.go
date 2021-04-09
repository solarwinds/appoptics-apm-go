// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import (
	"fmt"
	"testing"

	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanBaggageUnsampled(t *testing.T) {
	_ = reporter.SetTestReporter(reporter.TestReporterDisableDefaultSetting(true))
	tr := NewTracer()
	tr.(*Tracer).TrimUnsampledSpans = true
	span := tr.StartSpan("op")
	assert.NotNil(t, span)

	sp := span.SetBaggageItem("key", "val")
	assert.NotNil(t, sp)

	childSpan := tr.StartSpan("op2", opentracing.ChildOf(sp.Context()))
	assert.NotNil(t, childSpan)
}

func testTransactionName(t *testing.T, tagName, txnName string) {
	r := reporter.SetTestReporter() // set up test reporter
	tr := NewTracer()

	span := tr.StartSpan("op")
	assert.NotNil(t, span)
	span.SetTag(tagName, txnName)
	span.SetTag("http.url", "http://app.com/myURL/123/456")
	span.Finish()

	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeKVMap{
		{"op", "entry", "", ""}:                    {},
		{"op", "exit", "TransactionName", txnName}: {Edges: g.Edges{{"op", "entry"}}},
	})

	require.Len(t, r.SpanMessages, 1)
	m, ok := r.SpanMessages[0].(*metrics.HTTPSpanMessage)
	require.True(t, ok)
	assert.Equal(t, txnName, m.Transaction)
}

func TestOTSetTransactionName(t *testing.T) {
	testTransactionName(t, "TransactionName", "myTxn")
}

func TestOTSetResourceName(t *testing.T) {
	testTransactionName(t, "resource.name", "myTxn2")
}

type customStringer struct{}

func (customStringer) String() string { return "custom" }

type weirdType struct{}

func TestSetErrorTags(t *testing.T) {
	// test a bunch of different args to SetTag("error", ...) and how they show up in trace event KVs
	for _, tc := range []struct{ errorTagVal, errorClass, errorMsg interface{} }{
		{fmt.Errorf("An error!"), "*errors.errorString", "An error!"},
		{true, "error", "true"},
		{"error string", "error", "error string"},
		{customStringer{}, "error", "custom"},
		{weirdType{}, "error", "opentracing.weirdType"},
	} {
		t.Run(fmt.Sprintf("Error tagval %v, errClass %v, errMsg %v", tc.errorTagVal, tc.errorClass, tc.errorMsg), func(t *testing.T) {
			r := reporter.SetTestReporter() // set up test reporter
			tr := NewTracer()

			span := tr.StartSpan("op")
			assert.NotNil(t, span)
			span.SetTag("error", tc.errorTagVal)
			span.Finish()

			r.Close(3)
			g.AssertGraph(t, r.EventBufs, 3, g.AssertNodeMap{
				{"op", "entry"}: {},
				{"op", "error"}: {Edges: g.Edges{{"op", "entry"}}, Callback: func(n g.Node) {
					assert.Equal(t, tc.errorClass, n.Map["ErrorClass"])
					assert.Equal(t, tc.errorMsg, n.Map["ErrorMsg"])
				}},
				{"op", "exit"}: {Edges: g.Edges{{"op", "error"}}, Callback: func(n g.Node) {}},
			})
		})
	}

	// test a couple of cases where no error is reported
	for _, tc := range []struct{ errorTagVal interface{} }{
		{false},
		{nil},
	} {
		r := reporter.SetTestReporter() // set up test reporter
		tr := NewTracer()

		span := tr.StartSpan("op")
		assert.NotNil(t, span)
		span.SetTag("error", tc.errorTagVal)
		span.Finish()

		r.Close(2)
		g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
			{"op", "entry"}: {},
			{"op", "exit"}:  {Edges: g.Edges{{"op", "entry"}}, Callback: func(n g.Node) {}},
		})
	}
}

func TestHTTPSpanMetrics(t *testing.T) {
	r := reporter.SetTestReporter() // set up test reporter
	tr := NewTracer()

	span := tr.StartSpan("op")
	assert.NotNil(t, span)
	span.SetTag("error", true)
	span.SetTag("resource.name", "myTxn")
	span.SetTag("http.method", "PUT")
	span.SetTag("http.url", "http://domain/path/to/my/request")
	span.SetTag("http.status_code", "503")
	span.Finish()

	r.Close(3)
	g.AssertGraph(t, r.EventBufs, 3, g.AssertNodeMap{
		{"op", "entry"}: {},
		{"op", "error"}: {Edges: g.Edges{{"op", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, "error", n.Map["ErrorClass"])
			assert.Equal(t, "true", n.Map["ErrorMsg"])
		}},
		{"op", "exit"}: {Edges: g.Edges{{"op", "error"}}, Callback: func(n g.Node) {}},
	})

	require.Len(t, r.SpanMessages, 1)
	m, ok := r.SpanMessages[0].(*metrics.HTTPSpanMessage)
	assert.True(t, ok)
	assert.Equal(t, "myTxn", m.Transaction)
	assert.Equal(t, "/path/to/my/request", m.Path)
	assert.Equal(t, 503, m.Status)
	assert.Equal(t, "PUT", m.Method)
	assert.True(t, m.HasError)

	// test another span with no transaction name set and integer status_code
	{
		r := reporter.SetTestReporter()
		tr := NewTracer()

		span := tr.StartSpan("op")
		assert.NotNil(t, span)
		span.SetTag("http.method", "PUT").
			SetTag("http.url", "http://domain/path/to/my/request").
			SetTag("http.status_code", 503).
			Finish()

		r.Close(2)
		g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
			{"op", "entry"}: {},
			{"op", "exit"}:  {Edges: g.Edges{{"op", "entry"}}, Callback: func(n g.Node) {}},
		})

		require.Len(t, r.SpanMessages, 1)
		m, ok := r.SpanMessages[0].(*metrics.HTTPSpanMessage)
		assert.True(t, ok)
		assert.Equal(t, "/path/to", m.Transaction)
		assert.Equal(t, "/path/to/my/request", m.Path)
		assert.Equal(t, 503, m.Status)
		assert.Equal(t, "PUT", m.Method)
		assert.True(t, m.HasError)
	}
}
