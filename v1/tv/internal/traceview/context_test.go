// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"bytes"
	"crypto/rand"
	"errors"
	"reflect"
	"strings"
	"testing"

	g "github.com/appneta/go-appneta/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	// oboe_metadata_init
	// oboe_metadata_random
	var md1 oboeMetadata
	var mdNil *oboeMetadata
	assert.Panics(t, func() { mdNil.Init() })    // init nil md
	assert.Error(t, mdNil.SetRandom())           // random nil md
	md1.Init()                                   // init valid md
	assert.NoError(t, md1.SetRandom())           // make random md
	md1Str := md1.String()                       // get string repr of md
	t.Logf("md1: %s", md1Str)                    // log md string
	assert.Len(t, md1Str, oboeMetadataStringLen) // check metadata str len

	// oboe_metadata_pack
	buf := make([]byte, 64)
	assert.Equal(t, -1, oboeMetadataPack(nil, buf))            // pack nil md
	assert.Equal(t, -1, oboeMetadataPack(&md1, []byte("XXX"))) // pack valid md into too-small buf
	assert.Equal(t, 29, oboeMetadataPack(&md1, buf))           // pack valid md into valid buf

	// make metadata buf with bad header
	badVer := make([]byte, len(buf))
	copy(badVer, buf)
	badVer[0] = byte(0x2b)

	// oboe_metadata_unpack
	var mdUnpack oboeMetadata
	mdUnpack.Init()                                              // init new md
	assert.Equal(t, -1, oboeMetadataUnpack(nil, buf))            // unpack valid buf into nil md
	assert.Equal(t, -1, oboeMetadataUnpack(&mdUnpack, []byte{})) // unpack empty buf into md
	assert.Equal(t, -1, oboeMetadataUnpack(&mdUnpack, buf[:8]))  // unpack truncated buf into md
	assert.Equal(t, -2, oboeMetadataUnpack(&mdUnpack, badVer))   // unpack bad version buf into md
	assert.Equal(t, 0, oboeMetadataUnpack(&mdUnpack, buf))       // unpack valid buf into md
	assert.Equal(t, mdUnpack.String(), md1Str)                   // unpacked md string should match

	// oboe_metadata_pack for 12-byte shorter trace/task ID (default is 20 + 8-byte op ID)
	shortTaskLen := 12
	var mdS, mdSU oboeMetadata
	mdS.Init()                         // init regular metadata
	mdS.taskLen = shortTaskLen         // override task ID len
	assert.NoError(t, mdS.SetRandom()) // generate random task & op IDs
	bufS := make([]byte, 128)          // buffer to pack
	assert.Equal(t, (1 + shortTaskLen + 8),
		oboeMetadataPack(&mdS, bufS)) // pack buf
	mdSStr, err := oboeMetadataToString(&mdS) // encode as string
	assert.NoError(t, err)
	t.Logf("mdS: %s", mdSStr)                           // log 50 char hex string
	assert.Len(t, mdSStr, (1+shortTaskLen+8)*2)         // check len=(1 + 12 + 8)*2
	mdSU.taskLen = shortTaskLen                         // override target MD task len
	assert.Equal(t, 0, oboeMetadataUnpack(&mdSU, bufS)) // unpack
	assert.Equal(t, shortTaskLen, mdSU.taskLen)         // verify target MD task len
	assert.Equal(t, mdSStr, mdSU.String())              // verify unpacked value

	// oboe_metadata_fromstr
	var md2 oboeMetadata
	nullMd := "1B00000000000000000000000000000000000000000000000000000000"
	assert.NotEqual(t, md1Str, nullMd)                                // ensure md1 string is not null
	md2.Init()                                                        // init empty md2
	assert.Equal(t, nullMd, md2.String())                             // empty md produceds null md string
	assert.Equal(t, -1, oboeMetadataFromString(nil, md1Str))          // unpack str to nil md
	assert.Equal(t, -1, oboeMetadataFromString(&md2, "1BA70"))        // load md2 from invalid str
	assert.Equal(t, nullMd, md2.String())                             // no change to md2 from previous
	assert.Equal(t, -2, oboeMetadataFromString(&md2, "2"+md1Str[1:])) // load md2 from bad ver
	assert.Equal(t, nullMd, md2.String())                             // no change to md2 from previous
	assert.Equal(t, -1,
		oboeMetadataFromString(&md2, string(make([]byte, 2048)))) // load md2 from too-long string
	assert.Equal(t, nullMd, md2.String()) // no change to md2 from previous
	assert.Equal(t, -1,
		oboeMetadataFromString(&md2, strings.Replace(md1Str, "B", "Z", -1))) // load md2 from invalid hex
	assert.Equal(t, nullMd, md2.String())                    // no change to md2 from previous
	assert.Equal(t, 0, oboeMetadataFromString(&md2, md1Str)) // load md2 from valid hex
	assert.Equal(t, md1Str, md2.String())                    // md2 now should be same as md1

	// oboe_metadata_tostr
	assert.NotPanics(t, func() {
		str, e := oboeMetadataToString(nil) // convert nil to md str
		assert.Equal(t, str, "")            // shoud produce empty str
		assert.Error(t, e)                  // should raise error
	})
	s, err := oboeMetadataToString(&md2) // convert md2 to str
	assert.Equal(t, s, md1Str)           // assert matches md1 str
	assert.NoError(t, err)               // no error

	// context.String()
	ctx := &context{md2}
	assert.Equal(t, md1Str, ctx.String())
	nctx := &nullContext{}
	assert.Equal(t, "", nctx.String())

	// context.Copy()
	cctx := ctx.Copy().(*context)
	t.Logf("mdCopy: %v", cctx.String())
	assert.Equal(t, cctx.String(), ctx.String())
	assert.True(t, bytes.Equal(cctx.metadata.ids.taskID, ctx.metadata.ids.taskID))
	t.Logf("cctx opID %v", cctx.metadata.ids.opID)
	t.Logf(" ctx opID %v", ctx.metadata.ids.opID)
	assert.True(t, bytes.Equal(cctx.metadata.ids.opID, ctx.metadata.ids.opID))
	assert.Equal(t, ctx.metadata.taskLen, cctx.metadata.taskLen)
	assert.Equal(t, ctx.metadata.opLen, cctx.metadata.opLen)
	assert.Equal(t, len(cctx.metadata.ids.taskID), cctx.metadata.taskLen)
	assert.Equal(t, len(cctx.metadata.ids.opID), cctx.metadata.opLen)
	assert.Equal(t, len(ctx.metadata.ids.taskID), ctx.metadata.taskLen)
	assert.Equal(t, len(ctx.metadata.ids.opID), ctx.metadata.opLen)
	assert.Equal(t, oboeMetadataStringLen, len(cctx.String()))
}

type errorReader struct {
	failOn    map[int]bool
	callCount int
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	// fail always, or on specified calls
	r.callCount++
	if r.failOn == nil || r.failOn[r.callCount-1] {
		return 0, errors.New("rand error")
	}
	return rand.Read(p)
}

func TestMetadataRandom(t *testing.T) {
	r := SetTestReporter()
	// if RNG fails, don't report events/spans associated with RNG failures.
	randReader = &errorReader{failOn: map[int]bool{0: true}}
	ctx := newContext()
	assert.IsType(t, &nullContext{}, ctx)
	assert.Empty(t, r.Bufs) // no events reported

	// RNG failure on second call (for metadata op ID)
	randReader = &errorReader{failOn: map[int]bool{1: true}}
	ctx2 := newContext()
	assert.IsType(t, &nullContext{}, ctx2)
	assert.Empty(t, r.Bufs) // no events reported

	// RNG failure on third call (for event op ID)
	randReader = &errorReader{failOn: map[int]bool{2: true}}
	ctx3 := newContext()
	assert.IsType(t, ctx3, &context{}) // context created successfully
	e3 := ctx3.NewSampledEvent(LabelEntry, "randErrLayer", false)
	assert.IsType(t, &nullEvent{}, e3)
	assert.Empty(t, r.Bufs) // no events reported

	// RNG failure on valid context while trying to report an event
	randReader = &errorReader{failOn: map[int]bool{0: true}}
	assert.Error(t, ctx3.(*context).reportEvent(LabelEntry, "randErrLayer", false))
	assert.Empty(t, r.Bufs) // no events reported

	randReader = rand.Reader // set back to normal
}

// newTestContext returns a fresh random *context with no events reported for use in unit tests.
func newTestContext(t *testing.T) *context {
	ctx := newContext()
	assert.True(t, ctx.IsTracing())
	assert.IsType(t, ctx, &context{})
	return ctx.(*context)
}

func TestReportEventMap(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e, err := ctx.NewEvent(LabelEntry, "myLayer")
	assert.NoError(t, err)
	err = e.Report(ctx)
	assert.NoError(t, err)

	assert.NoError(t, ctx.ReportEventMap(LabelInfo, "myLayer", map[string]interface{}{
		"testK":  "testV",
		"intval": 333,
	}))
	g.AssertGraph(t, r.Bufs, 2, map[g.MatchNode]g.AssertNode{
		{"myLayer", "entry"}: {},
		{"myLayer", "info"}: {g.OutEdges{{"myLayer", "entry"}}, func(n g.Node) {
			assert.EqualValues(t, 333, n.Map["intval"])
			assert.Equal(t, "testV", n.Map["testK"])
		}},
	})
}

func TestNullContext(t *testing.T) {
	r := SetTestReporter()
	r.ShouldTrace = false

	ctx := NewContext("testLayer", "", false, nil) // nullContext{}
	assert.Equal(t, reflect.TypeOf(ctx).Elem().Name(), "nullContext")
	assert.False(t, ctx.IsTracing())
	assert.Empty(t, ctx.String())
	assert.False(t, ctx.Copy().IsTracing())
	// reporting shouldn't work
	assert.NoError(t, ctx.ReportEvent(LabelEntry, "testLayer"))
	assert.NoError(t, ctx.ReportEventMap(LabelInfo, "testLayer", map[string]interface{}{"K": "V"}))
	// try and make an event
	e := ctx.NewSampledEvent(LabelExit, "testLayer", false)
	assert.Empty(t, e.MetadataString())
	assert.NoError(t, e.ReportContext(ctx, false))
	assert.Len(t, r.Bufs, 0) // no reporting

	// try and report a real unrelated event on a null context
	e2, err := newTestContext(t).NewEvent(LabelEntry, "e2")
	assert.NoError(t, err)
	assert.NoError(t, e2.ReportContext(ctx, false))
	assert.Len(t, r.Bufs, 0) // no reporting

	// shouldn't be able to create a trace if the entry event fails
	r.ShouldTrace = true
	r.ShouldError = true
	ctxBad := NewContext("testBadEntry", "", true, nil)
	assert.Equal(t, reflect.TypeOf(ctxBad).Elem().Name(), "nullContext")

	assert.Len(t, r.Bufs, 0) // no reporting
}
