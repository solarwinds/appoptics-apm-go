// +build linux

package reporter

import (
	"strings"
	"syscall"
	"testing"

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
		sysname = Byte2String(uname.Sysname[:])
		version = Byte2String(uname.Version[:])
		sysname = strings.TrimRight(sysname, "\x00")
		version = strings.TrimRight(version, "\x00")
	}

	assert.Equal(t, sysname, m["UnameSysName"])
	assert.Equal(t, version, m["UnameVersion"])
}
