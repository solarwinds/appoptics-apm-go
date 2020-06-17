// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import (
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
