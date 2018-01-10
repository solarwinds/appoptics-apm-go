// +build linux

package traceview

import (
	"fmt"
	"os"
	"strconv"
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

func addHostMetrics(bbuf *bsonBuffer, index *int) {
	// system load of last minute
	if s := getStrByKeyword("/proc/loadavg", ""); s != "" {
		load, err := strconv.ParseFloat(strings.Fields(s)[0], 64)
		if err == nil {
			addMetricsValue(bbuf, index, "Load1", load)
		}
	}

	// system total memory
	if s := getStrByKeyword("/proc/meminfo", "MemTotal"); s != "" {
		memTotal := strings.Fields(s) // MemTotal: 7657668 kB
		if len(memTotal) == 3 {
			if total, err := strconv.Atoi(memTotal[1]); err == nil {
				addMetricsValue(bbuf, index, "TotalRAM", int64(total*1024))
			}
		}
	}

	// free memory
	if s := getStrByKeyword("/proc/meminfo", "MemFree"); s != "" {
		memFree := strings.Fields(s) // MemFree: 161396 kB
		if len(memFree) == 3 {
			if free, err := strconv.Atoi(memFree[1]); err == nil {
				addMetricsValue(bbuf, index, "FreeRAM", int64(free*1024)) // bytes
			}
		}
	}

	// process memory
	if s := getStrByKeyword("/proc/self/statm", ""); s != "" {
		processRAM := strings.Fields(s)
		if len(processRAM) != 0 {
			for _, ps := range processRAM {
				if p, err := strconv.Atoi(ps); err == nil {
					addMetricsValue(bbuf, index, "ProcessRAM", p*os.Getpagesize())
					break
				}
			}
		}
	}
}

// isPhysicalInterface returns true if the specified interface name is physical
func isPhysicalInterface(ifname string) bool {
	fn := "/sys/class/net/" + ifname
	link, err := os.Readlink(fn)
	if err != nil {
		OboeLog(ERROR, fmt.Sprintf("cannot readlink %s", fn))
		return false
	}
	if strings.Contains(link, "/virtual/") {
		return false
	}
	return true
}
