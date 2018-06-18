// Copyright (C) 2017 Librato, Inc. All rights reserved.

package agent

import (
	"bytes"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/stretchr/testify/assert"
)

func TestDebugLevel(t *testing.T) {
	tests := []struct {
		key      string
		val      string
		expected LogLevel
	}{
		{"APPOPTICS_DEBUG_LEVEL", "DEBUG", DEBUG},
		{"APPOPTICS_DEBUG_LEVEL", "Info", INFO},
		{"APPOPTICS_DEBUG_LEVEL", "warn", WARNING},
		{"APPOPTICS_DEBUG_LEVEL", "erroR", ERROR},
		{"APPOPTICS_DEBUG_LEVEL", "erroR  ", ERROR},
		{"APPOPTICS_DEBUG_LEVEL", "HelloWorld", _defaultLogLevel},
		{"APPOPTICS_DEBUG_LEVEL", "0", DEBUG},
		{"APPOPTICS_DEBUG_LEVEL", "1", INFO},
		{"APPOPTICS_DEBUG_LEVEL", "2", WARNING},
		{"APPOPTICS_DEBUG_LEVEL", "3", ERROR},
		{"APPOPTICS_DEBUG_LEVEL", "4", _defaultLogLevel},
		{"APPOPTICS_DEBUG_LEVEL", "1000", _defaultLogLevel},
	}

	for _, test := range tests {
		os.Setenv(test.key, test.val)
		Init()
		assert.EqualValues(t, test.expected, Level())
	}

	os.Unsetenv("APPOPTICS_DEBUG_LEVEL")
	Init()
	assert.EqualValues(t, Level(), _defaultLogLevel)
}

func TestLog(t *testing.T) {
	var buffer bytes.Buffer
	log.SetOutput(&buffer)

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "info")
	Init()

	tests := map[string]string{
		"hello world": "hello world\n",
		"":            "\n",
		"hello %s":    "hello %s\n",
	}

	for str, expected := range tests {
		buffer.Reset()
		Log(INFO, str)
		assert.True(t, strings.HasSuffix(buffer.String(), expected))
	}

	log.SetOutput(os.Stderr)
}

func TestStrToLevel(t *testing.T) {
	tests := map[string]LogLevel{
		"DEBUG": DEBUG,
		"INFO":  INFO,
		"WARN":  WARNING,
		"ERROR": ERROR,
	}
	for str, lvl := range tests {
		l, _ := StrToLevel(str)
		assert.Equal(t, lvl, l)
	}
}

func TestVerifyLogLevel(t *testing.T) {
	tests := map[string]LogLevel{
		"DEBUG":   DEBUG,
		"Debug":   DEBUG,
		"debug":   DEBUG,
		" dEbUg ": DEBUG,
		"INFO":    INFO,
		"WARN":    WARNING,
		"ERROR":   ERROR,
		"ABC":     _defaultLogLevel,
	}
	for str, lvl := range tests {
		assert.Equal(t, lvl, verifyLogLevel(str))
	}
}

func TestSetLevel(t *testing.T) {
	var wg = &sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(wg *sync.WaitGroup) {
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
			SetLevel(LogLevel(rand.Intn(len(levelStr))))
			Debug("hello world")
			wg.Done()
		}(wg)
	}
	wg.Wait()

	SetLevel(DEBUG)
	Error("", "one", "two", "three")
	assert.Equal(t, DEBUG, Level())
}
