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
	e, err := ctx.newEvent(LabelEntry, testLayer)
	assert.NoError(t, err)
	e.AddInt("IntTest", 123)

	err = e.Report(ctx)
	assert.NoError(t, err)

	err = ctx.ReportEvent("info", testLayer,
		"Controller", "test_controller",
		"Action", "test_action")
	assert.NoError(t, err)

	e, err = ctx.newEvent(LabelExit, testLayer)
	assert.NoError(t, err)
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
	var md, emptyMd oboeMetadata
	assert.Error(t, oboeEventInit(nil, nil))      // init nil evt, md
	assert.Error(t, oboeEventInit(evt, nil))      // init evt, nil md
	assert.Error(t, oboeEventInit(evt, &emptyMd)) // init evt, uninit'd md
	md.Init()                                     // init valid md
	assert.NoError(t, md.SetRandom())             // make random md
	t.Logf("TestEvent md: %v", md.String())
	assert.NoError(t, oboeEventInit(evt, &md))                 // init valid evt, md
	assert.Equal(t, evt.metadata.ids.taskID, md.ids.taskID)    // task IDs should match
	assert.NotEqual(t, evt.metadata.ids.opID, md.ids.opID)     // op IDs should not match
	assert.Len(t, evt.MetadataString(), oboeMetadataStringLen) // event md string correct length
}

func TestEventMetadata(t *testing.T) {
	r := SetTestReporter()

	ctx := newTestContext(t)
	e, err := ctx.newEvent(LabelExit, "alice")
	assert.NoError(t, err)
	e2, err := ctx.newEvent(LabelEntry, "bob")
	assert.NoError(t, err)

	ctx2 := newContext() // context for unassociated trace
	assert.Len(t, ctx.MetadataString(), oboeMetadataStringLen)
	assert.Len(t, ctx2.MetadataString(), oboeMetadataStringLen)
	assert.NotEqual(t, ctx.MetadataString()[2:42], ctx2.MetadataString()[2:42])
	// try to add ctx2 to to this event -- no effect
	e.AddEdgeFromMetadataString(ctx2.MetadataString())
	err = e.Report(ctx)
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
	e, err := ctx.newEvent(LabelEntry, testLayer)
	assert.NoError(t, err)
	err = e.Report(ctx)
	assert.NoError(t, err)
	// create SampledEvent with edge to entry
	se := ctx.NewEvent(LabelExit, testLayer, true)
	assert.NoError(t, se.ReportContext(ctx, false))

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {g.OutEdges{{"go_test", "entry"}}, nil},
	})
}
func TestSampledEventNoEdge(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e, err := ctx.newEvent(LabelEntry, testLayer)
	assert.NoError(t, err)
	err = e.Report(ctx)
	assert.NoError(t, err)
	se := ctx.NewEvent(LabelExit, testLayer, false) // create event without edge
	assert.NoError(t, se.ReportContext(ctx, false)) // report event without edge
	// exit event is unconnected
	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {},
	})
}
