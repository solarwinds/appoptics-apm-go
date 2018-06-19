// Copyright (C) 2017 Librato, Inc. All rights reserved.

package agent

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// LogLevel is a type that defines the log level.
type LogLevel uint8

// logLevel is the type for protected log level
// DO NOT COPY ME
type logLevel struct {
	LogLevel
	sync.RWMutex
}

// log levels
const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
)

// The string representation of log levels
var levelStr = []string{
	DEBUG:   "DEBUG",
	INFO:    "INFO",
	WARNING: "WARN",
	ERROR:   "ERROR",
}

// The global log level.
var (
	_defaultLogLevel, _ = StrToLevel(defaultLogLevel)
	_globalLevel        = &logLevel{LogLevel: _defaultLogLevel}
)

// SetLevel sets the log level of AppOptics agent
func (l *logLevel) SetLevel(level LogLevel) {
	l.Lock()
	defer l.Unlock()
	l.LogLevel = level
}

// Level returns the current log level of AppOptics agent
func (l *logLevel) Level() LogLevel {
	l.RLock()
	defer l.RUnlock()
	return l.LogLevel
}

var (
	SetLevel = _globalLevel.SetLevel
	Level    = _globalLevel.Level
)

// initLogging initializes the global logger with the configured log level
func initLogging() {
	SetLevel(verifyLogLevel(GetConfig(AppOpticsLogLevel)))
}

// verifyLogLevel verifies if a string correctly represents a valid log level and returns
// the level in LogLevel type. It will return the default level for invalid arguments
func verifyLogLevel(level string) (lvl LogLevel) {
	// We do not want to break backward-compatibility so keep accepting integer values.
	if i, err := strconv.Atoi(level); err == nil {
		// Protect the debug level from some invalid value, e.g., 1000
		if i < len(levelStr) {
			lvl = LogLevel(i)
		} else {
			lvl = _defaultLogLevel
		}

	} else if l, err := StrToLevel(strings.ToUpper(strings.TrimSpace(level))); err == nil {
		lvl = l
	} else {
		Warningf("invalid debug level: %s", level)
		lvl = _defaultLogLevel
	}
	return
}

// StrToLevel converts a log level in string format (e.g., "DEBUG") to the corresponding log level
// in LogLevel type. It returns ERROR (the highest level) and an error for invalid log level strings
func StrToLevel(e string) (LogLevel, error) {
	offset, err := elemOffset(levelStr, e)
	if err == nil {
		return LogLevel(offset), nil
	} else {
		return ERROR, err
	}
}

// elemOffset is a simple helper function to check if a slice contains a specific element
func elemOffset(s []string, e string) (int, error) {
	for idx, i := range s {
		if e == i {
			return idx, nil
		}
	}
	return -1, errors.New("not found")
}

// shouldLog checks if a message should be logged based on current level settings
func shouldLog(lv LogLevel) bool {
	return lv >= Level()
}

// logIt prints logs based on the debug level.
func logIt(level LogLevel, msg string, args []interface{}) {
	if !shouldLog(level) {
		return
	}

	var buffer bytes.Buffer
	const numberOfLayersToSkip = 2 // layer 1: logIt(), layer 2: its wrappers, e.g., Info()

	var pre string
	if level == DEBUG {
		// `runtime.Caller()` is called here to get the metadata of the caller of `Caller`:
		// the program counter, file name, and line number within the file of the corresponding call.
		// The argument `skip` is the number of stack frames to skip (for example, if skip == 0
		// you will always get the metadata of `logIt`, which is useless.)
		// skip = 2 is used here as there are wrappers on top of `logIt` (Info,
		// Infof, Error, etc). By skipping two layers (logIt and its wrapper), you may get
		// the information of real callers of the logging functions.
		pc, file, line, ok := runtime.Caller(numberOfLayersToSkip)
		if ok {
			path := strings.Split(runtime.FuncForPC(pc).Name(), ".")
			name := path[len(path)-1]
			pre = fmt.Sprintf("%s %s#%d %s(): ", levelStr[level], filepath.Base(file), line, name)
		} else {
			pre = fmt.Sprintf("%s %s#%s %s(): ", levelStr[level], "na", "na", "na")
		}
	} else { // avoid expensive reflections in production
		pre = fmt.Sprintf("%s ", levelStr[level])
	}

	buffer.WriteString(pre)

	s := msg
	if msg == "" {
		s = fmt.Sprint(args...)
	} else {
		s = fmt.Sprintf(msg, args...)
	}
	buffer.WriteString(s)

	log.Print(buffer.String())
}

// Logf formats the log message with specified args
// and print it in the specified level
func Logf(level LogLevel, msg string, args ...interface{}) {
	logIt(level, msg, args)
}

// Log prints the log message in the specified level
func Log(level LogLevel, args ...interface{}) {
	logIt(level, "", args)
}

// Debugf formats the log message with specified args
// and print it in the specified level
func Debugf(msg string, args ...interface{}) {
	logIt(DEBUG, msg, args)
}

// Debug prints the log message in the specified level
func Debug(args ...interface{}) {
	logIt(DEBUG, "", args)
}

// Infof formats the log message with specified args
// and print it in the specified level
func Infof(msg string, args ...interface{}) {
	logIt(INFO, msg, args)
}

// Info prints the log message in the specified level
func Info(args ...interface{}) {
	logIt(INFO, "", args)
}

// Warningf formats the log message with specified args
// and print it in the specified level
func Warningf(msg string, args ...interface{}) {
	logIt(WARNING, msg, args)
}

// Warning prints the log message in the specified level
func Warning(args ...interface{}) {
	logIt(WARNING, "", args)
}

// Errorf formats the log message with specified args
// and print it in the specified level
func Errorf(msg string, args ...interface{}) {
	logIt(ERROR, msg, args)
}

// Error prints the log message in the specified level
func Error(args ...interface{}) {
	logIt(ERROR, "", args)
}
