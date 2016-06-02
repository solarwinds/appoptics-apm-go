// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"math"
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

	intTest := int(57)
	int32Test := int32(59)
	int64Test := int64(60)
	uintTest := uint(61)
	uintBigTest := uint(math.MaxInt64 + 1)
	uint32Test := uint32(62)
	uint64Test := uint64(63)
	uint64BigTest := uint64(math.MaxInt64 + 1)
	float32Test := float32(0.1)
	float64Test := float64(3.5)
	stringTest := "testStr"
	boolTest := true
	bufTest := []byte("testBuf")
	badMD := "1DSF*&)DS&F#"

	err = ctx.ReportEvent("info", testLayer,
		"Controller", "test_controller",
		"Action", "test_action",
		"TestInt", &intTest,
		"TestInt32", &int32Test,
		"TestInt64", &int64Test,
		"TestUint", &uintTest,
		"TestUint32", &uint32Test,
		"TestUint64", &uint64Test,
		"TestUintBig", &uintBigTest,
		"TestUint64Big", &uint64BigTest,
		"TestUintV", uintTest,
		"TestUint32V", uint32Test,
		"TestUint64V", uint64Test,
		"TestUintBigV", uintBigTest,
		"TestUint64BigV", uint64BigTest,
		"TestFloat32", &float32Test,
		"TestFloat64", &float64Test,
		"TestString", &stringTest,
		"TestBool", &boolTest,
		"TestBuf", &bufTest,
		"Edge", &badMD, // should not be reported
		"Key", // key with no value -- should be skipped/ignored
	)
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
			assert.EqualValues(t, n.Map["TestInt"], intTest)
			assert.EqualValues(t, n.Map["TestInt32"], int32Test)
			assert.EqualValues(t, n.Map["TestInt64"], int64Test)
			assert.EqualValues(t, n.Map["TestUint"], uintTest)
			assert.EqualValues(t, n.Map["TestUint32"], uint32Test)
			assert.EqualValues(t, n.Map["TestUint64"], uint64Test)
			assert.NotContains(t, "TestUintBig", n.Map)
			assert.NotContains(t, "TestUint64Big", n.Map)
			assert.EqualValues(t, n.Map["TestUintV"], uintTest)
			assert.EqualValues(t, n.Map["TestUint32V"], uint32Test)
			assert.EqualValues(t, n.Map["TestUint64V"], uint64Test)
			assert.NotContains(t, "TestUintBigV", n.Map)
			assert.NotContains(t, "TestUint64BigV", n.Map)
			assert.Equal(t, n.Map["TestFloat32"], float64(float32Test))
			assert.Equal(t, n.Map["TestFloat64"], float64Test)
			assert.Equal(t, n.Map["TestString"], stringTest)
			assert.Equal(t, n.Map["TestBool"], boolTest)
			assert.Equal(t, n.Map["TestBuf"], bufTest)
		}},
		{"go_test", "exit"}: {g.OutEdges{{"go_test", "info"}}, nil},
	})
}

func TestOboeEvent(t *testing.T) {
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

func TestEvent(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e, err := ctx.newEvent(LabelEntry, testLayer)
	assert.NoError(t, err)
	err = e.Report(ctx)
	assert.NoError(t, err)
	// create Event with edge to entry
	se := ctx.NewEvent(LabelExit, testLayer, true)
	assert.NoError(t, se.ReportContext(ctx, false))

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"go_test", "entry"}: {},
		{"go_test", "exit"}:  {g.OutEdges{{"go_test", "entry"}}, nil},
	})
}
func TestEventNoEdge(t *testing.T) {
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
