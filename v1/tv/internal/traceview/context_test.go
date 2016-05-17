// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"strings"
	"testing"

	g "github.com/appneta/go-traceview/v1/tv/internal/graphtest"
	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	// oboe_metadata_init
	// oboe_metadata_random
	var md1 oboe_metadata_t
	assert.Equal(t, -1, oboe_metadata_init(nil))               // init nil md
	assert.NotPanics(t, func() { oboe_metadata_random(nil) })  // random nil md
	assert.Equal(t, 0, oboe_metadata_init(&md1))               // init valid md
	assert.NotPanics(t, func() { oboe_metadata_random(&md1) }) // make random md
	md1Str := MetadataString(&md1)                             // get string repr of md
	t.Logf("md1: %s", md1Str)                                  // log md string
	assert.Len(t, md1Str, 58)                                  // check metadata str len

	// oboe_metadata_pack
	buf := make([]byte, 64)
	assert.Equal(t, -1, oboe_metadata_pack(nil, buf))            // pack nil md
	assert.Equal(t, -1, oboe_metadata_pack(&md1, []byte("XXX"))) // pack valid md into too-small buf
	assert.Equal(t, 29, oboe_metadata_pack(&md1, buf))           // pack valid md into valid buf

	// make metadata buf with bad header
	badVer := make([]byte, len(buf))
	copy(badVer, buf)
	badVer[0] = byte(0x2b)

	// oboe_metadata_unpack
	var mdUnpack oboe_metadata_t
	assert.Equal(t, 0, oboe_metadata_init(&mdUnpack))              // init new md
	assert.Equal(t, -1, oboe_metadata_unpack(nil, buf))            // unpack valid buf into nil md
	assert.Equal(t, -1, oboe_metadata_unpack(&mdUnpack, []byte{})) // unpack empty buf into md
	assert.Equal(t, -1, oboe_metadata_unpack(&mdUnpack, buf[:8]))  // unpack truncated buf into md
	assert.Equal(t, -2, oboe_metadata_unpack(&mdUnpack, badVer))   // unpack bad version buf into md
	assert.Equal(t, 0, oboe_metadata_unpack(&mdUnpack, buf))       // unpack valid buf into md
	assert.Equal(t, MetadataString(&mdUnpack), md1Str)             // unpacked md string should match

	// oboe_metadata_fromstr
	var md2 oboe_metadata_t
	nullMd := "1B00000000000000000000000000000000000000000000000000000000"
	assert.NotEqual(t, md1Str, nullMd)                                                      // ensure md1 string is not null
	assert.Equal(t, 0, oboe_metadata_init(&md2))                                            // init empty md2
	assert.Equal(t, nullMd, MetadataString(&md2))                                           // empty md produceds null md string
	assert.Equal(t, -1, oboe_metadata_fromstr(nil, md1Str))                                 // unpack str to nil md
	assert.Equal(t, -1, oboe_metadata_fromstr(&md2, "1BA70"))                               // load md2 from invalid str
	assert.Equal(t, nullMd, MetadataString(&md2))                                           // no change to md2 from previous
	assert.Equal(t, -2, oboe_metadata_fromstr(&md2, "2"+md1Str[1:len(md1Str)]))             // load md2 from bad ver
	assert.Equal(t, nullMd, MetadataString(&md2))                                           // no change to md2 from previous
	assert.Equal(t, -1, oboe_metadata_fromstr(&md2, string(make([]byte, 2048))))            // load md2 from too-long string
	assert.Equal(t, nullMd, MetadataString(&md2))                                           // no change to md2 from previous
	assert.Equal(t, -1, oboe_metadata_fromstr(&md2, strings.Replace(md1Str, "B", "Z", -1))) // load md2 from invalid hex
	assert.Equal(t, nullMd, MetadataString(&md2))                                           // no change to md2 from previous
	assert.Equal(t, 0, oboe_metadata_fromstr(&md2, md1Str))                                 // load md2 from valid hex
	assert.Equal(t, md1Str, MetadataString(&md2))                                           // md2 now should be same as md1

	// oboe_metadata_tostr
	assert.NotPanics(t, func() {
		str, err := oboe_metadata_tostr(nil) // convert nil to md str
		assert.Equal(t, str, "")             // shoud produce empty str
		assert.Error(t, err)                 // should raise error
	})
	s, err := oboe_metadata_tostr(&md2) // convert md2 to str
	assert.Equal(t, s, md1Str)          // assert matches md1 str
	assert.NoError(t, err)              // no error

	// Context.String()
	ctx := &Context{md2}
	assert.Equal(t, md1Str, ctx.String())
	nctx := &NullContext{}
	assert.Equal(t, "", nctx.String())
}

func TestReportEventMap(t *testing.T) {
	r := SetTestReporter()
	ctx := NewContext()
	e := ctx.NewEvent(LabelEntry, "myLayer")
	err := e.Report(ctx)
	assert.NoError(t, err)

	ctx.ReportEventMap(LabelInfo, "myLayer", map[string]interface{}{
		"testK":  "testV",
		"intval": 333,
	})
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

	ctx := NewSampledContext("testLayer", "", false, nil) // nullContext{}
	assert.False(t, ctx.IsTracing())
	assert.Empty(t, ctx.String())
	assert.False(t, ctx.Copy().IsTracing())
	// reporting shouldn't work
	ctx.ReportEvent(LabelEntry, "testLayer")
	ctx.ReportEventMap(LabelInfo, "testLayer", map[string]interface{}{"K": "V"})
	// try and make an event
	e := ctx.NewSampledEvent(LabelExit, "testLayer", false)
	assert.Empty(t, e.MetadataString())
	e.ReportContext(ctx, false)

	assert.Len(t, r.Bufs, 0) // no reporting
}
