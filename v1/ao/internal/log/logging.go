// Copyright (C) 2017 Librato, Inc. All rights reserved.

// Package log implements a leveled logging system. It checks the current log
// level and decides whether to print the logging texts or ignore them.
package log

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
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

const (
	envAppOpticsLogLevel = "APPOPTICS_DEBUG_LEVEL"
)

// LevelStr represents the log levels in strings
var LevelStr = []string{
	DEBUG:   "DEBUG",
	INFO:    "INFO",
	WARNING: "WARN",
	ERROR:   "ERROR",
}

// DefaultLevel defines the default log level
const DefaultLevel = WARNING

// The global log level.
var (
	globalLevel = &logLevel{LogLevel: DefaultLevel}
	logger      = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)
)

func init() {
	SetLevelFromStr(os.Getenv(envAppOpticsLogLevel))
}

// SetOutput sets the output destination for the internal logger.
func SetOutput(w io.Writer) {
	logger.SetOutput(w)
}

// SetLevelFromStr parses the input string to a LogLevel and change the level of
// the global logger accordingly.
func SetLevelFromStr(s string) {
	level := DefaultLevel

	if l, valid := ToLogLevel(s); valid {
		level = l
	}

	SetLevel(level)
}

// ToLogLevel converts a string to a log level, or returns false for any error
func ToLogLevel(level string) (LogLevel, bool) {
	lvl := DefaultLevel

	// Accept integers for backward-compatibility.
	if i, err := strconv.Atoi(level); err == nil {
		if i < len(LevelStr) {
			lvl = LogLevel(i)
		} else {
			return lvl, false
		}
	} else {
		l, err := StrToLevel(strings.ToUpper(strings.TrimSpace(level)))
		if err == nil {
			lvl = l
		} else {
			return lvl, false
		}
	}
	return lvl, true
}

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
	// SetLevel is the wrapper for the global logger
	SetLevel = globalLevel.SetLevel

	// Level is the wrapper for the global logger
	Level = globalLevel.Level
)

// StrToLevel converts a log level in string format (e.g., "DEBUG") to the
// corresponding log level in LogLevel type. It returns ERROR (the highest
// level) and an error for invalid log level strings.
func StrToLevel(e string) (LogLevel, error) {
	offset, err := elemOffset(LevelStr, e)
	if err == nil {
		return LogLevel(offset), nil
	} else {
		return DefaultLevel, err
	}
}

// elemOffset is a simple helper function to check if a slice contains a
// specific element
func elemOffset(s []string, e string) (int, error) {
	for idx, i := range s {
		if e == i {
			return idx, nil
		}
	}
	return -1, errors.New("not found")
}

// shouldLog checks if a message should be logged with current level.
func shouldLog(lv LogLevel) bool {
	return lv >= Level()
}

// logIt prints logs based on the debug level.
func logIt(level LogLevel, msg string, args []interface{}) {
	if !shouldLog(level) {
		return
	}

	var buffer bytes.Buffer
	// layer 1: logIt(), layer 2: its wrappers, e.g., Info()
	const numberOfLayersToSkip = 2

	var pre string
	if level == DEBUG {
		// `runtime.Caller()` is called here to get the metadata of the caller of `Caller`:
		// the program counter, file name, and line number within the file of the corresponding call.
		// The argument `skip` is the number of stack frames to skip (for example, if skip == 0
		// you will always get the metadata of `logIt`, which is useless.)
		// skip = 2 is used here as there are wrappers on top of `logIt` (Info,
		// Infof, Error, etc). By skipping two layers (logIt and its wrapper), you may get
		// the information of real callers of the logging functions.
		_, file, line, ok := runtime.Caller(numberOfLayersToSkip)
		if ok {
			pre = fmt.Sprintf("%-5s [AO] %s:%d ", LevelStr[level], filepath.Base(file), line)
		} else {
			pre = fmt.Sprintf("%-5s [AO] %s:%s ", LevelStr[level], "na", "na")
		}
	} else { // avoid expensive reflections in production
		pre = fmt.Sprintf("%-5s [AO] ", LevelStr[level])
	}

	buffer.WriteString(pre)

	s := msg
	if msg == "" {
		s = fmt.Sprint(args...)
	} else {
		s = fmt.Sprintf(msg, args...)
	}
	buffer.WriteString(s)

	logger.Print(buffer.String())
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
