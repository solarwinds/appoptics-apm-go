// +build linux

// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"strings"
	"syscall"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/bson"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestAppendUname(t *testing.T) {
	bbuf := bson.NewBuffer()
	appendUname(bbuf)
	bbuf.Finish()
	m := bsonToMap(bbuf)

	var sysname, release string

	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err == nil {
		sysname = utils.Byte2String(uname.Sysname[:])
		release = utils.Byte2String(uname.Release[:])
		sysname = strings.TrimRight(sysname, "\x00")
		release = strings.TrimRight(release, "\x00")
	}

	assert.Equal(t, sysname, m["UnameSysName"])
	assert.Equal(t, release, m["UnameVersion"])
}
