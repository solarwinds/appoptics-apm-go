package agent

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// LogLevel is a type that defines the log level.
type LogLevel uint8

// log levels
const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
)

var logLevels = []string{
	DEBUG:   "DEBUG",
	INFO:    "INFO",
	WARNING: "WARN",
	ERROR:   "ERROR",
}

var logLevel = LogLevel(elemOffset(logLevels, strings.ToUpper(strings.TrimSpace(defaultLogLevel))))

func initLogging() {
	level := GetConfig(AppOpticsLogLevel)
	// We do not want to break backward-compatibility so keep accepting integer values.
	if i, err := strconv.Atoi(level); err == nil {
		// Protect the debug level from some invalid value, e.g., 1000
		if i >= len(logLevels) {
			i = len(logLevels) - 1
		}
		logLevel = LogLevel(i)
	} else if offset := elemOffset(logLevels, strings.ToUpper(strings.TrimSpace(level))); offset != -1 {
		logLevel = LogLevel(offset)
	} else {
		Warning("invalid debug level: %s", level)
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

// logit prints logs based on the debug level.
func logit(level LogLevel, msg string, args []interface{}) {
	if level < logLevel {
		return
	}

	var buffer bytes.Buffer
	pc, f, l, ok := runtime.Caller(1)
	if ok {
		path := strings.Split(runtime.FuncForPC(pc).Name(), ".")
		name := path[len(path)-1]
		buffer.WriteString(fmt.Sprintf("%s %s#%d %s(): ", logLevels[level], filepath.Base(f), l, name))
	} else {
		buffer.WriteString(fmt.Sprintf("%s %s#%s %s(): ", logLevels[level], "na", "na", "na"))
	}

	s := msg
	if msg == "" && len(args) > 0 {
		s = fmt.Sprint(args...)
	} else if msg != "" && len(args) > 0 {
		s = fmt.Sprintf(msg, args...)
	}
	buffer.WriteString(s)
	log.Print(buffer.String())
}

// Log prints the log message with specified level
func Log(level LogLevel, msg string, args ...interface{}) {
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
