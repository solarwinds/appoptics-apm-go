// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	g "github.com/appneta/go-traceview/v1/tv/internal/graphtest"
)

var test_layer string = "go_test"

func TestSendEvent(t *testing.T) {
	r := SetTestReporter()

	ctx := NewContext()
	e := ctx.NewEvent(LabelEntry, test_layer)
	e.AddInt("IntTest", 123)

	if err := e.Report(ctx); err != nil {
		t.Errorf("Unexpected %v", err)
	}

	ctx.ReportEvent("info", test_layer,
		"Controller", "test_controller",
		"Action", "test_action")

	e = ctx.NewEvent(LabelExit, test_layer)
	e.AddEdge(ctx)
	if err := e.Report(ctx); err != nil {
		t.Errorf("Unexpected %v", err)
	}

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
