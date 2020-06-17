// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"context"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"github.com/appoptics/appoptics-apm-go/v1/ao/http"
	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/stretchr/testify/assert"
)

func TestTraceMetadata(t *testing.T) {
	r := reporter.SetTestReporter()

	tr := ao.NewTrace("test")
	md := tr.ExitMetadata()

	// verify loggable trace ID
	assert.True(t, strings.HasSuffix(tr.LoggableTraceID(), "-1"))
	assert.Equal(t, 42, len(tr.LoggableTraceID()))

	tr.End("Edge", "872453", // bad Edge KV, should be ignored
		"NotReported") // odd-length arg, should be ignored

	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"test", "entry"}: {},
		{"test", "exit"}: {Edges: g.Edges{{"test", "entry"}}, Callback: func(n g.Node) {
			// exit event should match ExitMetadata
			assert.Equal(t, md, n.Map[http.XTraceHeader])
		}},
	})
}

func TestNoTraceMetadata(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterDisableTracing())

	// if trace is not sampled, metadata should be empty
	tr := ao.NewTrace("test")

	// verify loggable trace ID
	assert.True(t, strings.HasSuffix(tr.LoggableTraceID(), "-0"))
	assert.Equal(t, 42, len(tr.LoggableTraceID()))

	md := tr.ExitMetadata()
	tr.EndCallback(func() ao.KVMap { return ao.KVMap{"Not": "reported"} })

	assert.NotEqual(t, "", md)
	assert.Len(t, r.EventBufs, 0)
}

// ensure two different traces have different trace IDs
func TestTraceMetadataDiff(t *testing.T) {
	r := reporter.SetTestReporter()

	t1 := ao.NewTrace("test1")
	md1 := t1.ExitMetadata()
	assert.Len(t, md1, 60)
	t1.End()
	r.Close(2)
	assert.Len(t, r.EventBufs, 2)

	r = reporter.SetTestReporter()
	t2 := ao.NewTrace("test1")
	md2 := t2.ExitMetadata()
	assert.Len(t, md2, 60)
	md2b := t2.ExitMetadata()
	md2c := t2.ExitMetadata()
	t2.End()
	r.Close(2)
	assert.Len(t, r.EventBufs, 2)

	assert.NotEqual(t, md1, md2)
	assert.NotEqual(t, md1[2:42], md2[2:42])

	// ensure that additional calls to ExitMetadata produce the same result
	assert.Len(t, md2b, 60)
	assert.Len(t, md2c, 60)
	assert.Equal(t, md2, md2b)
	assert.Equal(t, md2b, md2c)

	// OK to get exit metadata after trace ends, but should also be same
	md2d := t2.ExitMetadata()
	assert.Equal(t, md2d, md2c)
}

// example trace
func traceExample(t *testing.T, ctx context.Context) {
	// do some work
	f0(ctx)

	tr := ao.FromContext(ctx)
	// instrument a DB query
	q := "SELECT * FROM tbl"
	// l, _ := ao.BeginSpan(ctx, "DBx", "Query", q, "Flavor", "postgresql", "RemoteHost", "db.com")
	l := ao.BeginQuerySpan(ctx, "DBx", q, "postgresql", "db.com")
	// db.Query(q)
	time.Sleep(20 * time.Millisecond)
	l.Error("QueryError", "Error running query!")
	l.End()

	// ao.Info and ao.Error report on the root span
	tr.Info("HTTP-Status", 500)
	tr.Error("TimeoutError", "response timeout")

	// end the trace
	tr.End()
}

// example trace
func traceExampleCtx(t *testing.T, ctx context.Context) {
	// do some work
	f0Ctx(ctx)

	// instrument a DB query
	q := []byte("SELECT * FROM tbl")
	_, ctxQ := ao.BeginSpan(ctx, "DBx", "Query", q, "Flavor", "postgresql", "RemoteHost", "db.com")
	assert.True(t, ao.IsSampled(ctxQ))
	// db.Query(q)
	time.Sleep(20 * time.Millisecond)
	ao.Error(ctxQ, "QueryError", "Error running query!")
	ao.End(ctxQ)
	assert.False(t, ao.IsSampled(ctxQ)) // Not considered sampled after span ends

	// ao.Info and ao.Error report on the root span
	ao.Info(ctx, "HTTP-Status", 500)
	ao.Error(ctx, "TimeoutError", "response timeout")

	// end the trace
	ao.EndTrace(ctx)
}

// example work function
func f0(ctx context.Context) {
	// 	l, _ := ao.BeginSpan(ctx, "http.Get", "RemoteURL", "http://a.b")
	l := ao.BeginRemoteURLSpan(ctx, "http.Get", "http://a.b")
	time.Sleep(5 * time.Millisecond)
	// _, _ = http.Get("http://a.b")

	// test reporting a variety of value types
	l.Info("floatV", 3.5, "boolT", true, "boolF", false, "bigV", 5000000000,
		"int64V", int64(5000000001), "int32V", int32(100), "float32V", float32(0.1),
		// test reporting an unsupported type -- currently will be silently ignored
		"weirdType", func() {},
	)
	// test reporting a non-string key: should not work, won't report any events
	l.Info(3, "3")

	time.Sleep(5 * time.Millisecond)
	l.Err(errors.New("test error!"))
	l.End()
}

// example work function
func f0Ctx(ctx context.Context) {
	_, ctx = ao.BeginSpan(ctx, "http.Get", "RemoteURL", "http://a.b")
	time.Sleep(5 * time.Millisecond)
	// _, _ = http.Get("http://a.b")

	// test reporting a variety of value types
	ao.Info(ctx, "floatV", 3.5, "boolT", true, "boolF", false, "bigV", 5000000000,
		"int64V", int64(5000000001), "int32V", int32(100), "float32V", float32(0.1),
		// test reporting an unsupported type -- currently will be silently ignored
		"weirdType", func() {},
	)
	// test reporting a non-string key: should not work, won't report any events
	ao.Info(ctx, 3, "3")

	time.Sleep(5 * time.Millisecond)
	ao.Err(ctx, errors.New("test error!"))
	ao.End(ctx)
}

func TestTraceExample(t *testing.T) {
	r := reporter.SetTestReporter() // enable test reporter
	// create a new trace, and a context to carry it around
	ctx := ao.NewContext(context.Background(), ao.NewTrace("myExample"))
	t.Logf("Reporting unrecognized event KV type")
	traceExample(t, ctx) // generate events
	r.Close(11)
	assertTraceExample(t, "f0", r.EventBufs)
}

func TestTraceExampleCtx(t *testing.T) {
	r := reporter.SetTestReporter() // enable test reporter
	// create a new trace, and a context to carry it around
	ctx := ao.NewContext(context.Background(), ao.NewTrace("myExample"))
	t.Logf("Reporting unrecognized event KV type")
	traceExampleCtx(t, ctx) // generate events
	r.Close(11)
	assertTraceExample(t, "f0Ctx", r.EventBufs)
}

func assertTraceExample(t *testing.T, f0name string, bufs [][]byte) {
	g.AssertGraph(t, bufs, 11, g.AssertNodeMap{
		// entry event should have no edges
		{"myExample", "entry"}: {Callback: func(n g.Node) {
			h, err := os.Hostname()
			assert.NoError(t, err)
			assert.Equal(t, h, n.Map["Hostname"])
		}},
		// nested span in http.Get profile points to trace entry
		{"http.Get", "entry"}: {Edges: g.Edges{{"myExample", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["RemoteURL"], "http://a.b")
		}},
		// http.Get info points to entry
		{"http.Get", "info"}: {Edges: g.Edges{{"http.Get", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, n.Map["floatV"], 3.5)
			assert.Equal(t, n.Map["boolT"], true)
			assert.Equal(t, n.Map["boolF"], false)
			assert.EqualValues(t, n.Map["bigV"], 5000000000)
			assert.EqualValues(t, n.Map["int64V"], 5000000001)
			assert.EqualValues(t, n.Map["int32V"], 100)
			assert.EqualValues(t, n.Map["float32V"], float32(0.1))
		}},
		// http.Get error points to info
		{"http.Get", "error"}: {Edges: g.Edges{{"http.Get", "info"}}, Callback: func(n g.Node) {
			assert.Equal(t, "error", n.Map["ErrorClass"])
			assert.Equal(t, "test error!", n.Map["ErrorMsg"])
		}},
		// end of nested span should link to last span event (error)
		{"http.Get", "exit"}: {Edges: g.Edges{{"http.Get", "error"}}},
		// first query after call to f0 should link to ...?
		{"DBx", "entry"}: {Edges: g.Edges{{"myExample", "entry"}}, Callback: func(n g.Node) {
			assert.EqualValues(t, n.Map["Query"], "SELECT * FROM tbl")
			assert.Equal(t, n.Map["Flavor"], "postgresql")
			assert.Equal(t, n.Map["RemoteHost"], "db.com")
		}},
		// error in nested span should link to span entry
		{"DBx", "error"}: {Edges: g.Edges{{"DBx", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, "QueryError", n.Map["ErrorClass"])
			assert.Equal(t, "Error running query!", n.Map["ErrorMsg"])
		}},
		// end of nested span should link to span entry
		{"DBx", "exit"}: {Edges: g.Edges{{"DBx", "error"}}},

		{"myExample", "info"}: {Edges: g.Edges{{"myExample", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, 500, n.Map["HTTP-Status"])
		}},
		{"myExample", "error"}: {Edges: g.Edges{{"myExample", "info"}}, Callback: func(n g.Node) {
			assert.Equal(t, "TimeoutError", n.Map["ErrorClass"])
			assert.Equal(t, "response timeout", n.Map["ErrorMsg"])
		}},
		{"myExample", "exit"}: {Edges: g.Edges{
			{"http.Get", "exit"}, {"DBx", "exit"}, {"myExample", "error"},
		}},
	})
}
func TestNoTraceExample(t *testing.T) {
	r := reporter.SetTestReporter()
	ctx := context.Background()
	traceExample(t, ctx)
	assert.False(t, ao.IsSampled(ctx))
	assert.Len(t, r.EventBufs, 0)
}

func BenchmarkNewTrace(b *testing.B) {
	_ = reporter.SetTestReporter(reporter.TestReporterShouldTrace(false))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ao.NewTrace("test")
	}
}

func BenchmarkNewTraceFromID(b *testing.B) {
	_ = reporter.SetTestReporter(reporter.TestReporterShouldTrace(false))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ao.NewTraceFromID("test", "", nil)
	}
}

func TestTraceFromMetadata(t *testing.T) {
	r := reporter.SetTestReporter()

	// emulate incoming request with X-Trace header
	incomingID := "2BF4CAA9299299E3D38A58A9821BD34F6268E576CFAB2198D447EA220301"
	tr := ao.NewTraceFromID("test", incomingID, nil)
	// verify loggable trace ID
	assert.Equal(t, "F4CAA9299299E3D38A58A9821BD34F6268E576CF-1", tr.LoggableTraceID())
	tr.EndCallback(func() ao.KVMap { return ao.KVMap{"Extra": "Arg"} })

	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have edge to incoming opID
		{"test", "entry"}: {Edges: g.Edges{{"Edge", incomingID[42:58]}}, Callback: func(n g.Node) {
			// trace ID should match incoming ID
			assert.Equal(t, incomingID[2:42], n.Map[http.XTraceHeader].(string)[2:42])
		}},
		// exit event links to entry
		{"test", "exit"}: {Edges: g.Edges{{"test", "entry"}}, Callback: func(n g.Node) {
			// trace ID should match incoming ID
			assert.Equal(t, incomingID[2:42], n.Map[http.XTraceHeader].(string)[2:42])
			assert.Equal(t, "Arg", n.Map["Extra"])
		}},
	})
}
func TestNoTraceFromMetadata(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterDisableTracing())
	tr := ao.NewTraceFromID("test", "", nil)
	md := tr.ExitMetadata()
	tr.End()

	assert.NotEqual(t, "", md)
	assert.Len(t, r.EventBufs, 0)
}
func TestNoTraceFromBadMetadata(t *testing.T) {
	r := reporter.SetTestReporter(reporter.TestReporterDisableTracing())

	// emulate incoming request with invalid X-Trace header
	incomingID := "1BF4CAA9299299E3D38A58A9821BD34F6268E576CFAB2A2203"
	tr := ao.NewTraceFromID("test", incomingID, nil)
	// verify loggable trace ID
	assert.True(t, strings.HasSuffix(tr.LoggableTraceID(), "-0"))
	assert.Equal(t, 42, len(tr.LoggableTraceID()))

	md := tr.ExitMetadata()
	tr.End("Edge", "823723875") // should not report
	assert.NotEqual(t, "", md)
	assert.Len(t, r.EventBufs, 0)
}

func TestTraceJoin(t *testing.T) {
	r := reporter.SetTestReporter()

	tr := ao.NewTrace("test")
	l := tr.BeginSpan("L1")
	l.End()
	tr.End()

	r.Close(4)
	g.AssertGraph(t, r.EventBufs, 4, g.AssertNodeMap{
		// entry event should have no edges
		{"test", "entry"}: {},
		{"L1", "entry"}:   {Edges: g.Edges{{"test", "entry"}}},
		{"L1", "exit"}:    {Edges: g.Edges{{"L1", "entry"}}},
		{"test", "exit"}:  {Edges: g.Edges{{"L1", "exit"}, {"test", "entry"}}},
	})
}

func TestNullTrace(t *testing.T) {
	r := reporter.SetTestReporter()
	tr := ao.NewNullTrace()
	md := tr.ExitMetadata()
	tr.End()
	assert.Equal(t, md, "")
	assert.Len(t, r.EventBufs, 0)
}

func TestTraceWithOptions(t *testing.T) {
	r := reporter.SetTestReporter()

	tr := ao.NewTraceWithOptions("test", ao.SpanOptions{})
	tr.End()

	tr = ao.NewTraceWithOptions("testWithBacktrace", ao.SpanOptions{WithBackTrace: true})
	tr.End()

	r.Close(4)
	g.AssertGraph(t, r.EventBufs, 4, g.AssertNodeMap{
		// entry event should have no edges
		{"test", "entry"}: {},
		{"test", "exit"}: {Edges: g.Edges{{"test", "entry"}}, Callback: func(n g.Node) {
			assert.Nil(t, n.Map[ao.KeyBackTrace])
		}},
		{"testWithBacktrace", "entry"}: {Callback: func(n g.Node) {
			assert.NotNil(t, n.Map[ao.KeyBackTrace])
		}},
		{"testWithBacktrace", "exit"}: {Edges: g.Edges{{"testWithBacktrace", "entry"}}},
	})
}
