// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"github.com/stretchr/testify/assert"
	"testing"
	//"labix.org/v2/mgo/bson"
)

func TestFlushBSON(t *testing.T) {
	debugLog = true
	debugLevel = DEBUG
	r := SetGRPCTestReporter()
	assert.IsType(t, &grpcReporter{}, r)
	// Process the raw metrics records and generate BSON message.
	ctx := newTestContext(t)
	e, err := ctx.newEvent(LabelEntry, testLayer)
	assert.NoError(t, err)
	e.AddInt("IntTest", 123)

	e, err = ctx.newEvent(LabelExit, testLayer)

	bArr, err := r.(*grpcReporter).mAgg.FlushBSON(newDefaultSettings())
	assert.NoError(t, err)
	//var m map[string]interface{}
	//bson.Unmarshal(bArr, m)
	//t.Log(string(bArr))
	assert.NotEqual(t, 10, len(bArr))
}
