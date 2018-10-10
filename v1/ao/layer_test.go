package ao

import (
	"context"
	"fmt"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestErrorSpec(t *testing.T) {
	r := reporter.SetTestReporter()

	tr := NewTrace("test")
	tr.Error("testClass", "testMsg")
	tr.End()

	r.Close(3)

	var foundErrEvt = false
	for _, evt := range r.EventBufs {
		m := make(map[string]interface{})
		bson.Unmarshal(evt, m)
		if m["Label"] != reporter.LabelError {
			continue
		}
		foundErrEvt = true
		// check error spec
		assert.Equal(t, reporter.LabelError, m["Label"])
		assert.Equal(t, "test", m["Layer"])
		assert.Equal(t, "error", m["Spec"])
		assert.Equal(t, "testMsg", m["ErrorMsg"])
		assert.Contains(t, m, "Timestamp_u")
		assert.Contains(t, m, "X-Trace")
		assert.Contains(t, m, KeyBackTrace)
		assert.Equal(t, "1", m["_V"])
		assert.Equal(t, "testClass", m["ErrorClass"])
		assert.IsType(t, "", m[KeyBackTrace])
	}
	assert.True(t, foundErrEvt)
}

func TestBeginSpan(t *testing.T) {
	r := reporter.SetTestReporter()

	ctx := NewContext(context.Background(), NewTrace("baseSpan"))
	s, _ := BeginSpan(ctx, "testSpan")

	subSpan, _ := BeginSpanWithOptions(ctx, "spanWithBackTrace", SpanOptions{WithBackTrace: true})
	subSpan.BeginSpanWithOptions("spanWithBackTrace", SpanOptions{WithBackTrace: true}).End()
	subSpan.End()

	s.End()
	EndTrace(ctx)

	r.Close(8)

	for _, evt := range r.EventBufs {
		m := make(map[string]interface{})
		bson.Unmarshal(evt, m)
		if m["Label"] != "entry" {
			continue
		}
		layer := m["Layer"]
		switch layer {
		case "testSpan":
			assert.Nil(t, m[KeyBackTrace], layer)
		case "spanWithBackTrace":
			assert.NotNil(t, m[KeyBackTrace], layer)
		case "spanWithBackTrace2":
			assert.NotNil(t, m[KeyBackTrace], layer)
		}
	}
}

func TestSpanInfo(t *testing.T) {
	r := reporter.SetTestReporter()

	ctx := NewContext(context.Background(), NewTrace("baseSpan"))
	s, _ := BeginSpan(ctx, "testSpan")

	s.InfoWithOptions(SpanOptions{WithBackTrace: true})

	s.End()
	EndTrace(ctx)

	r.Close(5)

	for _, evt := range r.EventBufs {
		m := make(map[string]interface{})
		bson.Unmarshal(evt, m)
		if m["Label"] != "info" {
			continue
		}
		layer := m["Layer"]
		switch layer {
		case "testSpan":
			assert.NotNil(t, m[KeyBackTrace], layer)
		}
	}
}

func TestFromKVs(t *testing.T) {
	assert.Equal(t, 0, len(fromKVs()))
	assert.Equal(t, 0, len(fromKVs("hello")))
	m := fromKVs("hello", 1)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, 1, m["hello"])

	m = fromKVs("hello", 1, 2)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, 1, m["hello"])

	m = fromKVs("hello", 1, "world", 2)
	assert.Equal(t, 2, len(m))
	assert.Equal(t, 1, m["hello"])
	assert.Equal(t, 2, m["world"])

	m = fromKVs(1, 1, 2)
	assert.Equal(t, 0, len(m))

	m = fromKVs(nil, "hello", 1)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, 1, m["hello"])

	m = fromKVs(1.1, "hello", 1)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, 1, m["hello"])

	m = fromKVs([]string{"1", "2"}, "hello", 1)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, 1, m["hello"])
}

func TestAddKVsFromOpts(t *testing.T) {
	assert.Equal(t, 0, len(addKVsFromOpts(SpanOptions{})))

	kvs := addKVsFromOpts(SpanOptions{}, "hello")
	assert.Equal(t, []interface{}{"hello"}, kvs)

	kvs = addKVsFromOpts(SpanOptions{}, "hello", 1)
	assert.Equal(t, []interface{}{"hello", 1}, kvs)

	kvs = addKVsFromOpts(SpanOptions{WithBackTrace: true})
	assert.Equal(t, 2, len(kvs))

	kvs = addKVsFromOpts(SpanOptions{WithBackTrace: true}, "hello", 1)
	assert.Equal(t, 4, len(kvs))
}

func TestMergeKVs(t *testing.T) {
	cases := []struct {
		left   []interface{}
		right  []interface{}
		merged []interface{}
	}{
		{[]interface{}{}, []interface{}{}, []interface{}{}},
		{[]interface{}{"a"}, []interface{}{}, []interface{}{"a"}},
		{[]interface{}{}, []interface{}{"a"}, []interface{}{"a"}},
		{[]interface{}{"a"}, []interface{}{"b"}, []interface{}{"a", "b"}},
		{[]interface{}{"a", "b"}, []interface{}{"c", "d"}, []interface{}{"a", "b", "c", "d"}},
	}

	for idx, c := range cases {
		assert.Equal(t, c.merged, mergeKVs(c.left, c.right), fmt.Sprintf("Test case: #%d", idx))
	}
}
