// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/appneta/go-appneta/v1/tv"
	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/appneta/go-appneta/v1/tv/internal/traceview"
	"github.com/stretchr/testify/assert"
)

func TestTraceMetadata(t *testing.T) {
	r := traceview.SetTestReporter()

	tr := tv.NewTrace("test")
	md := tr.ExitMetadata()
	tr.End("Edge", "872453", // bad Edge KV, should be ignored
		"NotReported") // odd-length arg, should be ignored

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"test", "entry"}: {},
		{"test", "exit"}: {g.OutEdges{{"test", "entry"}}, func(n g.Node) {
			// exit event should match ExitMetadata
			assert.Equal(t, md, n.Map["X-Trace"])
		}},
	})
}
func TestNoTraceMetadata(t *testing.T) {
	r := traceview.SetTestReporter()
	r.ShouldTrace = false

	// if trace is not sampled, metadata should be empty
	tr := tv.NewTrace("test")
	md := tr.ExitMetadata()
	tr.EndCallback(func() tv.KVMap { return tv.KVMap{"Not": "reported"} })

	assert.Equal(t, md, "")
	assert.Len(t, r.Bufs, 0)
}

// ensure two different traces have different trace IDs
func TestTraceMetadataDiff(t *testing.T) {
	r := traceview.SetTestReporter()

	t1 := tv.NewTrace("test1")
	md1 := t1.ExitMetadata()
	assert.Len(t, md1, 58)
	t1.End()
	assert.Len(t, r.Bufs, 2)

	t2 := tv.NewTrace("test1")
	md2 := t2.ExitMetadata()
	assert.Len(t, md2, 58)
	md2b := t2.ExitMetadata()
	md2c := t2.ExitMetadata()
	t2.End()
	assert.Len(t, r.Bufs, 4)

	assert.NotEqual(t, md1, md2)
	assert.NotEqual(t, md1[2:42], md2[2:42])

	// ensure that additional calls to ExitMetadata produce the same result
	assert.Len(t, md2b, 58)
	assert.Len(t, md2c, 58)
	assert.Equal(t, md2, md2b)
	assert.Equal(t, md2b, md2c)

	// OK to get exit metadata after trace ends, but should also be same
	md2d := t2.ExitMetadata()
	assert.Equal(t, md2d, md2c)
}

// example trace
func traceExample(ctx context.Context) {
	// do some work
	f0(ctx)

	t := tv.FromContext(ctx)
	// instrument a DB query
	q := "SELECT * FROM tbl"
	// l, _ := tv.BeginLayer(ctx, "DBx", "Query", q, "Flavor", "postgresql", "RemoteHost", "db.com")
	l := tv.BeginQueryLayer(ctx, "DBx", q, "postgresql", "db.com")
	// db.Query(q)
	time.Sleep(20 * time.Millisecond)
	l.Error("QueryError", "Error running query!")
	l.End()

	// tv.Info and tv.Error report on the root span
	t.Info("HTTP-Status", 500)
	t.Error("TimeoutError", "response timeout")

	// end the trace
	t.End()
}

// example trace
func traceExampleCtx(ctx context.Context) {
	// do some work
	f0Ctx(ctx)

	// instrument a DB query
	q := []byte("SELECT * FROM tbl")
	_, ctxQ := tv.BeginLayer(ctx, "DBx", "Query", q, "Flavor", "postgresql", "RemoteHost", "db.com")
	// db.Query(q)
	time.Sleep(20 * time.Millisecond)
	tv.Error(ctxQ, "QueryError", "Error running query!")
	tv.End(ctxQ)

	// tv.Info and tv.Error report on the root span
	tv.Info(ctx, "HTTP-Status", 500)
	tv.Error(ctx, "TimeoutError", "response timeout")

	// end the trace
	tv.EndTrace(ctx)
}

// example work function
func f0(ctx context.Context) {
	defer tv.BeginProfile(ctx, "f0").End()

	//	l, _ := tv.BeginLayer(ctx, "http.Get", "RemoteURL", "http://a.b")
	l := tv.BeginRemoteURLLayer(ctx, "http.Get", "http://a.b")
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
	defer tv.BeginProfile(ctx, "f0").End()

	_, ctx = tv.BeginLayer(ctx, "http.Get", "RemoteURL", "http://a.b")
	time.Sleep(5 * time.Millisecond)
	// _, _ = http.Get("http://a.b")

	// test reporting a variety of value types
	tv.Info(ctx, "floatV", 3.5, "boolT", true, "boolF", false, "bigV", 5000000000,
		"int64V", int64(5000000001), "int32V", int32(100), "float32V", float32(0.1),
		// test reporting an unsupported type -- currently will be silently ignored
		"weirdType", func() {},
	)
	// test reporting a non-string key: should not work, won't report any events
	tv.Info(ctx, 3, "3")

	time.Sleep(5 * time.Millisecond)
	tv.Err(ctx, errors.New("test error!"))
	tv.End(ctx)
}

func TestTraceExample(t *testing.T) {
	r := traceview.SetTestReporter() // enable test reporter
	// create a new trace, and a context to carry it around
	ctx := tv.NewContext(context.Background(), tv.NewTrace("myExample"))
	t.Logf("Reporting unrecognized event KV type")
	traceExample(ctx) // generate events
	assertTraceExample(t, "f0", r.Bufs)
}

func TestTraceExampleCtx(t *testing.T) {
	r := traceview.SetTestReporter() // enable test reporter
	// create a new trace, and a context to carry it around
	ctx := tv.NewContext(context.Background(), tv.NewTrace("myExample"))
	t.Logf("Reporting unrecognized event KV type")
	traceExampleCtx(ctx) // generate events
	assertTraceExample(t, "f0Ctx", r.Bufs)
}

func assertTraceExample(t *testing.T, f0name string, bufs [][]byte) {
	g.AssertGraph(t, bufs, 13, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"myExample", "entry"}: {nil, func(n g.Node) {
			h, err := os.Hostname()
			assert.NoError(t, err)
			assert.Equal(t, h, n.Map["Hostname"])
		}},
		// first profile event should link to entry event
		{"", "profile_entry"}: {g.OutEdges{{"myExample", "entry"}}, func(n g.Node) {
			assert.Equal(t, n.Map["Language"], "go")
			assert.Equal(t, n.Map["ProfileName"], "f0")
			assert.Equal(t, n.Map["FunctionName"], "github.com/appneta/go-appneta/v1/tv_test."+f0name)
		}},
		{"", "profile_exit"}: {g.OutEdges{{"", "profile_entry"}}, nil},
		// nested layer in http.Get profile points to trace entry
		{"http.Get", "entry"}: {g.OutEdges{{"myExample", "entry"}}, func(n g.Node) {
			assert.Equal(t, n.Map["RemoteURL"], "http://a.b")
		}},
		// http.Get info points to entry
		{"http.Get", "info"}: {g.OutEdges{{"http.Get", "entry"}}, func(n g.Node) {
			assert.Equal(t, n.Map["floatV"], 3.5)
			assert.Equal(t, n.Map["boolT"], true)
			assert.Equal(t, n.Map["boolF"], false)
			assert.EqualValues(t, n.Map["bigV"], 5000000000)
			assert.EqualValues(t, n.Map["int64V"], 5000000001)
			assert.EqualValues(t, n.Map["int32V"], 100)
			assert.EqualValues(t, n.Map["float32V"], float32(0.1))
		}},
		// http.Get error points to info
		{"http.Get", "error"}: {g.OutEdges{{"http.Get", "info"}}, func(n g.Node) {
			assert.Equal(t, "error", n.Map["ErrorClass"])
			assert.Equal(t, "test error!", n.Map["ErrorMsg"])
		}},
		// end of nested layer should link to last layer event (error)
		{"http.Get", "exit"}: {g.OutEdges{{"http.Get", "error"}}, nil},
		// first query after call to f0 should link to ...?
		{"DBx", "entry"}: {g.OutEdges{{"myExample", "entry"}}, func(n g.Node) {
			assert.EqualValues(t, n.Map["Query"], "SELECT * FROM tbl")
			assert.Equal(t, n.Map["Flavor"], "postgresql")
			assert.Equal(t, n.Map["RemoteHost"], "db.com")
		}},
		// error in nested layer should link to layer entry
		{"DBx", "error"}: {g.OutEdges{{"DBx", "entry"}}, func(n g.Node) {
			assert.Equal(t, "QueryError", n.Map["ErrorClass"])
			assert.Equal(t, "Error running query!", n.Map["ErrorMsg"])
		}},
		// end of nested layer should link to layer entry
		{"DBx", "exit"}: {g.OutEdges{{"DBx", "error"}}, nil},

		{"myExample", "info"}: {g.OutEdges{{"myExample", "entry"}}, func(n g.Node) {
			assert.Equal(t, 500, n.Map["HTTP-Status"])
		}},
		{"myExample", "error"}: {g.OutEdges{{"myExample", "info"}}, func(n g.Node) {
			assert.Equal(t, "TimeoutError", n.Map["ErrorClass"])
			assert.Equal(t, "response timeout", n.Map["ErrorMsg"])
		}},
		{"myExample", "exit"}: {g.OutEdges{
			{"http.Get", "exit"}, {"", "profile_exit"}, {"DBx", "exit"}, {"myExample", "error"},
		}, nil},
	})
}
func TestNoTraceExample(t *testing.T) {
	r := traceview.SetTestReporter()
	ctx := context.Background()
	traceExample(ctx)
	assert.Len(t, r.Bufs, 0)
}

func BenchmarkNewTrace(b *testing.B) {
	r := traceview.SetTestReporter()
	r.ShouldTrace = false
	for i := 0; i < b.N; i++ {
		//_ = tv.NewTraceFromID("test", "", nil)
		_ = tv.NewTrace("test")
	}
}

func TestTraceFromMetadata(t *testing.T) {
	r := traceview.SetTestReporter()

	// emulate incoming request with X-Trace header
	incomingID := "1BF4CAA9299299E3D38A58A9821BD34F6268E576CFAB2198D447EA2203"
	tr := tv.NewTraceFromID("test", incomingID, nil)
	tr.EndCallback(func() tv.KVMap { return tv.KVMap{"Extra": "Arg"} })

	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		// entry event should have edge to incoming opID
		{"test", "entry"}: {g.OutEdges{{"Edge", incomingID[42:]}}, func(n g.Node) {
			// trace ID should match incoming ID
			assert.Equal(t, incomingID[2:42], n.Map["X-Trace"].(string)[2:42])
		}},
		// exit event links to entry
		{"test", "exit"}: {g.OutEdges{{"test", "entry"}}, func(n g.Node) {
			// trace ID should match incoming ID
			assert.Equal(t, incomingID[2:42], n.Map["X-Trace"].(string)[2:42])
			assert.Equal(t, "Arg", n.Map["Extra"])
		}},
	})
}
func TestNoTraceFromMetadata(t *testing.T) {
	r := traceview.SetTestReporter()
	r.ShouldTrace = false
	tr := tv.NewTraceFromID("test", "", nil)
	md := tr.ExitMetadata()
	tr.End()

	assert.Equal(t, md, "")
	assert.Len(t, r.Bufs, 0)
}
func TestTraceFromBadMetadata(t *testing.T) {
	r := traceview.SetTestReporter()

	// emulate incoming request with invalad X-Trace header
	incomingID := "1BF4CAA9299299E3D38A58A9821BD34F6268E576CFAB2A2203"
	tr := tv.NewTraceFromID("test", incomingID, nil)
	md := tr.ExitMetadata()
	tr.End("Edge", "823723875") // should not report
	assert.Equal(t, md, "")
	assert.Len(t, r.Bufs, 0)
}

func TestTraceJoin(t *testing.T) {
	r := traceview.SetTestReporter()

	tr := tv.NewTrace("test")
	l := tr.BeginLayer("L1")
	l.End()
	tr.End()

	g.AssertGraph(t, r.Bufs, 4, map[g.MatchNode]g.AssertNode{
		// entry event should have no edges
		{"test", "entry"}: {},
		{"L1", "entry"}:   {g.OutEdges{{"test", "entry"}}, nil},
		{"L1", "exit"}:    {g.OutEdges{{"L1", "entry"}}, nil},
		{"test", "exit"}:  {g.OutEdges{{"L1", "exit"}, {"test", "entry"}}, nil},
	})
}
