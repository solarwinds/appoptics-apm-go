// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"testing"

	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

var testLayer = "go_test"

func TestSendEvent(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e := ctx.NewEvent(LabelEntry, testLayer)
	e.AddInt("IntTest", 123)

	err := e.Report(ctx)
	assert.NoError(t, err)

	err = ctx.ReportEvent("info", testLayer,
		"Controller", "test_controller",
		"Action", "test_action")
	assert.NoError(t, err)

	e = ctx.NewEvent(LabelExit, testLayer)
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
	evt := &event{}
	var md oboeMetadata
	assert.Equal(t, -1, oboeEventInit(nil, nil)) // init nil evt, md
	assert.Equal(t, -1, oboeEventInit(evt, nil)) // init evt, nil md
	assert.Equal(t, 0, oboeMetadataInit(&md))    // init valid md
	assert.NoError(t, md.SetRandom())            // make random md
	t.Logf("TestEvent md: %v", md.String())
	assert.Equal(t, 0, oboeEventInit(evt, &md))                // init valid evt, md
	assert.Equal(t, evt.metadata.ids.taskID, md.ids.taskID)    // task IDs should match
	assert.NotEqual(t, evt.metadata.ids.opID, md.ids.opID)     // op IDs should not match
	assert.Len(t, evt.MetadataString(), oboeMetadataStringLen) // event md string correct length
}

func TestEventMetadata(t *testing.T) {
	r := SetTestReporter()

	ctx := newTestContext(t)
	e := ctx.NewEvent(LabelExit, "alice")
	e2 := ctx.NewEvent(LabelEntry, "bob")

	ctx2 := newContext() // context for unassociated trace
	assert.Len(t, ctx.String(), oboeMetadataStringLen)
	assert.Len(t, ctx2.String(), oboeMetadataStringLen)
	assert.NotEqual(t, ctx.String()[2:42], ctx2.String()[2:42])
	// try to add ctx2 to to this event -- no effect
	e.AddEdgeFromMetadataString(ctx2.String())
	err := e.Report(ctx)
	assert.NoError(t, err)
	// try to add e to e2 -- should work
	e2.AddEdgeFromMetadataString(e.MetadataString())
	assert.NoError(t, e2.Report(ctx))
	// test event pair
	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"alice", "exit"}: {},
		{"bob", "entry"}:  {g.OutEdges{{"alice", "exit"}}, nil},
	})
}

func TestSampledEvent(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e := ctx.NewEvent(LabelEntry, testLayer)
	err := e.Report(ctx)
	assert.NoError(t, err)
	// create SampledEvent with edge to entry
	se := ctx.NewSampledEvent(LabelExit, testLayer, true)
	assert.NoError(t, se.ReportContext(ctx, false))

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {g.OutEdges{{"go_test", "entry"}}, nil},
	})
}
func TestSampledEventNoEdge(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e := ctx.NewEvent(LabelEntry, testLayer)
	err := e.Report(ctx)
	assert.NoError(t, err)
	se := ctx.NewSampledEvent(LabelExit, testLayer, false) // create event without edge
	assert.NoError(t, se.ReportContext(ctx, false))        // report event without edge
	// exit event is unconnected
	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {},
	})
}
