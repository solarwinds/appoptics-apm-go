package agent

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// DebugLevel is a type that defines the log level.
type DebugLevel uint8

// log levels
const (
	DEBUG DebugLevel = iota
	INFO
	WARNING
	ERROR
)

var dbgLevels = []string{
	DEBUG:   "DEBUG",
	INFO:    "INFO",
	WARNING: "WARN",
	ERROR:   "ERROR",
}

var debugLevel = DebugLevel(elemOffset(dbgLevels, strings.ToUpper(strings.TrimSpace(defaultDebugLevel))))

func initLogging() {
	level := GetConfig(AppOpticsDebugLevel)
	// We do not want to break backward-compatibility so keep accepting integer values.
	if i, err := strconv.Atoi(level); err == nil {
		// Protect the debug level from some invalid value, e.g., 1000
		if i >= len(dbgLevels) {
			i = len(dbgLevels) - 1
		}
		debugLevel = DebugLevel(i)
	} else if offset := elemOffset(dbgLevels, strings.ToUpper(strings.TrimSpace(level))); offset != -1 {
		debugLevel = DebugLevel(offset)
	} else {
		Log(WARNING, fmt.Sprintf("invalid debug level: %s", level))
	}
}

// elemOffset is a simple helper function to check if a slice contains a specific element
func elemOffset(s []string, e string) int {
	for idx, i := range s {
		if e == i {
			return idx
		}
	}
	return -1
}

// log print logs based on the debug level.
func logit(level DebugLevel, msg string, args []interface{}) {
	if level < debugLevel {
		return
	}
	var p string
	pc, f, l, ok := runtime.Caller(1)
	if ok {
		path := strings.Split(runtime.FuncForPC(pc).Name(), ".")
		name := path[len(path)-1]
		p = fmt.Sprintf("%s %s#%d %s(): ", dbgLevels[level], filepath.Base(f), l, name)
	} else {
		p = fmt.Sprintf("%s %s#%s %s(): ", dbgLevels[level], "na", "na", "na")
	}

	s := msg
	if msg == "" && len(args) > 0 {
		s = fmt.Sprint(args...)
	} else if msg != "" && len(args) > 0 {
		s = fmt.Sprintf(msg, args...)
	}
	log.Print(p + s)
}

// Log prints the log message with specified level
func Log(level DebugLevel, msg string, args ...interface{}) {
	logit(level, msg, args)
}

// Debug prints the log message in DEBUG level
func Debug(msg string, args ...interface{}) {
	logit(DEBUG, msg, args)
}

// Info prints the log message in INFO level
func Info(msg string, args ...interface{}) {
	logit(INFO, msg, args)
}

// Warning prints the log message in WARNING level
func Warning(msg string, args ...interface{}) {
	logit(WARNING, msg, args)
}

// Error prints the log message in ERROR level
func Error(msg string, args ...interface{}) {
	logit(ERROR, msg, args)
}
