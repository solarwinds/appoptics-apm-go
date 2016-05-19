// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"testing"

	g "github.com/appneta/go-traceview/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

var test_layer = "go_test"

func TestSendEvent(t *testing.T) {
	r := SetTestReporter()

	ctx := NewContext()
	e := ctx.NewEvent(LabelEntry, test_layer)
	e.AddInt("IntTest", 123)

	err := e.Report(ctx)
	assert.NoError(t, err)

	err = ctx.ReportEvent("info", test_layer,
		"Controller", "test_controller",
		"Action", "test_action")
	assert.NoError(t, err)

	e = ctx.NewEvent(LabelExit, test_layer)
	e.AddEdge(ctx)
	err = e.Report(ctx)
	assert.NoError(t, err)

	g.AssertGraph(t, r.Bufs, 3, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {nil, func(n g.Node) {
			assert.EqualValues(t, n.Map["IntTest"], 123)
		}},
		{"go_test", "info"}: {g.OutEdges{{"go_test", "entry"}}, func(n g.Node) {
			assert.Equal(t, n.Map["Controller"], "test_controller")
			assert.Equal(t, n.Map["Action"], "test_action")
		}},
		{"go_test", "exit"}: {g.OutEdges{{"go_test", "info"}}, nil},
	})
}

func TestEvent(t *testing.T) {
	// oboe_event_init
	evt := &Event{}
	var md oboe_metadata_t
	assert.Equal(t, -1, oboe_event_init(nil, nil))            // init nil evt, md
	assert.Equal(t, -1, oboe_event_init(evt, nil))            // init evt, nil md
	assert.Equal(t, 0, oboe_metadata_init(&md))               // init valid md
	assert.NotPanics(t, func() { oboe_metadata_random(&md) }) // make random md
	t.Logf("TestEvent md: %v", MetadataString(&md))
	assert.Equal(t, 0, oboe_event_init(evt, &md))             // init valid evt, md
	assert.Equal(t, evt.metadata.ids.task_id, md.ids.task_id) // task IDs should match
	assert.NotEqual(t, evt.metadata.ids.op_id, md.ids.op_id)  // op IDs should not match
	assert.Len(t, evt.MetadataString(), 58)                   // event md string correct length
}

func TestEventMetadata(t *testing.T) {
	r := SetTestReporter()

	ctx := NewContext()
	e := ctx.NewEvent(LabelExit, "alice")
	e2 := ctx.NewEvent(LabelEntry, "bob")

	ctx2 := NewContext() // context for unassociated trace
	assert.Len(t, ctx.String(), 58)
	assert.Len(t, ctx2.String(), 58)
	assert.NotEqual(t, ctx.String()[2:42], ctx2.String()[2:42])
	// try to add ctx2 to to this event -- no effect
	e.AddEdgeFromMetadataString(ctx2.String())
	err := e.Report(ctx)
	assert.NoError(t, err)
	// try to add e to e2 -- should work
	e2.AddEdgeFromMetadataString(e.MetadataString())
	e2.Report(ctx)
	// test event pair
	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"alice", "exit"}: {},
		{"bob", "entry"}:  {g.OutEdges{{"alice", "exit"}}, nil},
	})
}

func TestSampledEvent(t *testing.T) {
	r := SetTestReporter()
	ctx := NewContext()
	e := ctx.NewEvent(LabelEntry, test_layer)
	err := e.Report(ctx)
	assert.NoError(t, err)
	// create SampledEvent with edge to entry
	se := ctx.NewSampledEvent(LabelExit, test_layer, true)
	se.ReportContext(ctx, false)

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {g.OutEdges{{"go_test", "entry"}}, nil},
	})
}
func TestSampledEventNoEdge(t *testing.T) {
	r := SetTestReporter()
	ctx := NewContext()
	e := ctx.NewEvent(LabelEntry, test_layer)
	err := e.Report(ctx)
	assert.NoError(t, err)
	se := ctx.NewSampledEvent(LabelExit, test_layer, false) // create event without edge
	se.ReportContext(ctx, false)                            // report event without edge
	// exit event is unconnected
	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {},
	})
}
