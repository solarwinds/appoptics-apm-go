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
	assert.NotPanics(t, func() { mdNil.Init() }) // init nil md
	assert.Error(t, mdNil.SetRandom())           // random nil md
	md1.Init()                                   // init valid md
	assert.NoError(t, md1.SetRandom())           // make random md
	md1Str := md1.String()                       // get string repr of md
	t.Logf("md1: %s", md1Str)                    // log md string
	assert.Len(t, md1Str, oboeMetadataStringLen) // check metadata str len

	// oboe_metadata_pack
	buf := make([]byte, 64)
	smallBuf := make([]byte, 3)
	pkcnt, pkerr := mdNil.Pack(buf) // pack nil md
	assert.Equal(t, 0, pkcnt)
	assert.Error(t, pkerr)
	pkcnt, pkerr = md1.Pack(smallBuf) // pack valid md into too-small buf
	assert.Equal(t, 0, pkcnt)
	assert.Error(t, pkerr)
	pkcnt, pkerr = md1.Pack(buf) // pack valid md into valid buf
	assert.Equal(t, 29, pkcnt)
	assert.NoError(t, pkerr)

	// make metadata buf with bad header
	badVer := make([]byte, len(buf))
	copy(badVer, buf)
	badVer[0] = byte(0x2b)

	// oboe_metadata_unpack
	var mdUnpack oboeMetadata
	assert.NotPanics(t, func() { mdNil.Init() })
	mdUnpack.Init()                            // init new md
	assert.Error(t, mdNil.Unpack(buf))         // unpack valid buf into nil md
	assert.Error(t, mdUnpack.Unpack([]byte{})) // unpack empty buf into md
	assert.Error(t, mdUnpack.Unpack(buf[:8]))  // unpack truncated buf into md
	assert.Error(t, mdUnpack.Unpack(badVer))   // unpack bad version buf into md
	assert.NoError(t, mdUnpack.Unpack(buf))    // unpack valid buf into md
	assert.Equal(t, mdUnpack.String(), md1Str) // unpacked md string should match

	// oboe_metadata_pack for 12-byte shorter trace/task ID (default is 20 + 8-byte op ID)
	shortTaskLen := 12
	var mdS, mdSU oboeMetadata
	mdS.Init()                         // init regular metadata
	mdS.taskLen = shortTaskLen         // override task ID len
	assert.NoError(t, mdS.SetRandom()) // generate random task & op IDs
	bufS := make([]byte, 128)          // buffer to pack
	pkcnt, pkerr = mdS.Pack(bufS)
	assert.NoError(t, pkerr)
	assert.Equal(t, (1 + shortTaskLen + 8), pkcnt) // pack buf
	mdSStr, err := mdS.ToString()                  // encode as string
	assert.NoError(t, err)
	t.Logf("mdS: %s", mdSStr)                   // log 50 char hex string
	assert.Len(t, mdSStr, (1+shortTaskLen+8)*2) // check len=(1 + 12 + 8)*2
	mdSU.taskLen = shortTaskLen                 // override target MD task len
	assert.NoError(t, mdSU.Unpack(bufS))        // unpack
	assert.Equal(t, shortTaskLen, mdSU.taskLen) // verify target MD task len
	assert.Equal(t, mdSStr, mdSU.String())      // verify unpacked value

	var mdPE oboeMetadata           // pack error
	mdPEStr, err := mdPE.ToString() // encode uninit md
	assert.Error(t, err)
	assert.Empty(t, mdPEStr)

	// oboe_metadata_fromstr
	var md2 oboeMetadata
	nullMd := "1B00000000000000000000000000000000000000000000000000000000"
	assert.NotEqual(t, md1Str, nullMd)                          // ensure md1 string is not null
	md2.Init()                                                  // init empty md2
	assert.Equal(t, nullMd, md2.String())                       // empty md produces null md string
	assert.Error(t, mdNil.FromString(md1Str))                   // unpack str to nil md
	assert.Error(t, md2.FromString("1BA70"))                    // load md2 from invalid str
	assert.Equal(t, nullMd, md2.String())                       // no change to md2 from previous
	assert.Error(t, md2.FromString("2"+md1Str[1:]))             // load md2 from bad ver
	assert.Equal(t, nullMd, md2.String())                       // no change to md2 from previous
	assert.Error(t, md2.FromString(string(make([]byte, 2048)))) // load md2 from too-long string
	assert.Equal(t, nullMd, md2.String())                       // no change to md2 from previous
	assert.Error(t,
		md2.FromString(strings.Replace(md1Str, "B", "Z", -1))) // load md2 from invalid hex
	assert.Equal(t, nullMd, md2.String())     // no change to md2 from previous
	assert.NoError(t, md2.FromString(md1Str)) // load md2 from valid hex
	assert.Equal(t, md1Str, md2.String())     // md2 now should be same as md1

	// oboe_metadata_tostr
	assert.NotPanics(t, func() {
		s, e := mdNil.ToString() // convert nil to md str
		assert.Equal(t, s, "")   // shoud produce empty str
		assert.Error(t, e)       // should raise error
	})
	s, err := md2.ToString()   // convert md2 to str
	assert.Equal(t, s, md1Str) // assert matches md1 str
	assert.NoError(t, err)     // no error

	// context.String()
	ctx := &oboeContext{md2}
	assert.Equal(t, md1Str, ctx.MetadataString())
	nctx := &nullContext{}
	assert.Equal(t, "", nctx.MetadataString())

	// context.Copy()
	cctx := ctx.Copy().(*oboeContext)
	t.Logf("mdCopy: %v", cctx.MetadataString())
	assert.Equal(t, cctx.MetadataString(), ctx.MetadataString())
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
	assert.Equal(t, oboeMetadataStringLen, len(cctx.MetadataString()))
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
	assert.IsType(t, ctx3, &oboeContext{}) // context created successfully
	e3 := ctx3.NewEvent(LabelEntry, "randErrLayer", false)
	assert.IsType(t, &nullEvent{}, e3)
	assert.Empty(t, r.Bufs) // no events reported

	// RNG failure on valid context while trying to report an event
	randReader = &errorReader{failOn: map[int]bool{0: true}}
	assert.Error(t, ctx3.(*oboeContext).reportEvent(LabelEntry, "randErrLayer", false))
	assert.Empty(t, r.Bufs) // no events reported

	randReader = rand.Reader // set back to normal
}

// newTestContext returns a fresh random *context with no events reported for use in unit tests.
func newTestContext(t *testing.T) *oboeContext {
	ctx := newContext()
	assert.True(t, ctx.IsTracing())
	assert.IsType(t, ctx, &oboeContext{})
	return ctx.(*oboeContext)
}

func TestReportEventMap(t *testing.T) {
	r := SetTestReporter()
	ctx := newTestContext(t)
	e, err := ctx.newEvent(LabelEntry, "myLayer")
	assert.NoError(t, err)
	err = e.Report(ctx)
	assert.NoError(t, err)

	assert.NoError(t, ctx.ReportEventMap(LabelInfo, "myLayer", map[string]interface{}{
		"testK":  "testV",
		"intval": 333,
	}))
	g.AssertGraph(t, r.Bufs, 2, g.AssertNodeMap{
		{"myLayer", "entry"}: {},
		{"myLayer", "info"}: {OutEdges: g.OutEdges{{"myLayer", "entry"}}, Callback: func(n g.Node) {
			assert.EqualValues(t, 333, n.Map["intval"])
			assert.Equal(t, "testV", n.Map["testK"])
		}},
	})
}

func TestNewContext(t *testing.T) {
	r := SetTestReporter()
	r.ShouldTrace = true
	ctx, ok := NewContext("testBadMd", "hello", true, nil) // test invalid metadata string
	assert.False(t, ok)
	assert.Equal(t, reflect.TypeOf(ctx).Elem().Name(), "nullContext")
	assert.Len(t, r.Bufs, 0) // no reporting
}

func TestNullContext(t *testing.T) {
	r := SetTestReporter()
	r.ShouldTrace = false

	ctx, ok := NewContext("testLayer", "", false, nil) // nullContext{}
	assert.False(t, ok)
	assert.Equal(t, reflect.TypeOf(ctx).Elem().Name(), "nullContext")
	assert.False(t, ctx.IsTracing())
	assert.Empty(t, ctx.MetadataString())
	assert.False(t, ctx.Copy().IsTracing())
	// reporting shouldn't work
	assert.NoError(t, ctx.ReportEvent(LabelEntry, "testLayer"))
	assert.NoError(t, ctx.ReportEventMap(LabelInfo, "testLayer", map[string]interface{}{"K": "V"}))
	// try and make an event
	e := ctx.NewEvent(LabelExit, "testLayer", false)
	assert.Empty(t, e.MetadataString())
	assert.NoError(t, e.ReportContext(ctx, false))
	assert.Len(t, r.Bufs, 0) // no reporting

	// try and report a real unrelated event on a null context
	e2, err := newTestContext(t).newEvent(LabelEntry, "e2")
	assert.NoError(t, err)
	assert.NoError(t, e2.ReportContext(ctx, false))
	assert.Len(t, r.Bufs, 0) // no reporting

	// shouldn't be able to create a trace if the entry event fails
	r.ShouldTrace = true
	r.ShouldError = true
	ctxBad, ok := NewContext("testBadEntry", "", true, nil)
	assert.False(t, ok)
	assert.Equal(t, reflect.TypeOf(ctxBad).Elem().Name(), "nullContext")

	assert.Len(t, r.Bufs, 0) // no reporting
}
