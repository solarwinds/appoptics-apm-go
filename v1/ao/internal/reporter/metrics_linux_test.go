// +build linux

// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"strings"
	"syscall"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestAppendUname(t *testing.T) {
	bbuf := NewBsonBuffer()
	appendUname(bbuf)
	bsonBufferFinish(bbuf)
	m := bsonToMap(bbuf)

	var sysname, version string

	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err == nil {
		sysname = utils.Byte2String(uname.Sysname[:])
		version = utils.Byte2String(uname.Version[:])
		sysname = strings.TrimRight(sysname, "\x00")
		version = strings.TrimRight(version, "\x00")
	}

	assert.Equal(t, sysname, m["UnameSysName"])
	assert.Equal(t, version, m["UnameVersion"])
}
