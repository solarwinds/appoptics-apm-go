// +build linux

package traceview

import (
	"strings"
	"syscall"
)

// gets and appends UnameSysName/UnameVersion to a BSON buffer
// bbuf	the BSON buffer to append the KVs to
func appendUname(bbuf *bsonBuffer) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err == nil {
		sysname := Byte2String(uname.Sysname[:])
		version := Byte2String(uname.Version[:])
		bsonAppendString(bbuf, "UnameSysName", strings.TrimRight(sysname, "\x00"))
		bsonAppendString(bbuf, "UnameVersion", strings.TrimRight(version, "\x00"))
	}
}
