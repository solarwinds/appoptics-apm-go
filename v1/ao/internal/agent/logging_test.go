// Copyright (C) 2017 Librato, Inc. All rights reserved.

package agent

import (
	"os"
	"testing"

	"log"

	"bytes"

	"strings"

	"math/rand"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDebugLevel(t *testing.T) {
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "DEBUG")
	Init()
	assert.EqualValues(t, Level(), LogLevel(0))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "Info")
	Init()
	assert.EqualValues(t, Level(), LogLevel(1))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "warn")
	Init()
	assert.EqualValues(t, Level(), LogLevel(2))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "erroR")
	Init()
	assert.EqualValues(t, Level(), LogLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", " erroR  ")
	Init()
	assert.EqualValues(t, Level(), LogLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "HelloWorld")
	Init()
	assert.EqualValues(t, Level(), _defaultLogLevel)

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "0")
	Init()
	assert.EqualValues(t, Level(), LogLevel(0))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "1")
	Init()
	assert.EqualValues(t, Level(), LogLevel(1))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "2")
	Init()
	assert.EqualValues(t, Level(), LogLevel(2))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "3")
	Init()
	assert.EqualValues(t, Level(), LogLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "4")
	Init()
	assert.EqualValues(t, Level(), _defaultLogLevel)

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "1000")
	Init()
	assert.EqualValues(t, Level(), _defaultLogLevel)

	os.Unsetenv("APPOPTICS_DEBUG_LEVEL")
	Init()
	assert.EqualValues(t, Level(), _defaultLogLevel)
}

func TestLog(t *testing.T) {
	var buffer bytes.Buffer

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "info")
	Init()
	log.SetOutput(&buffer)
	str := "hello world"
	Log(INFO, str)
	assert.True(t, strings.HasSuffix(buffer.String(), str+"\n"))

	buffer.Reset()
	str = ""
	Log(INFO, "")
	assert.True(t, strings.HasSuffix(buffer.String(), str+"\n"))

	buffer.Reset()
	str = ""
	Log(INFO, "", nil)
	assert.True(t, strings.HasSuffix(buffer.String(), str+"\n"))

	buffer.Reset()
	str = "hello %s"
	Log(INFO, "hello %s", nil)
	assert.True(t, strings.HasSuffix(buffer.String(), "hello %!s(<nil>)"+"\n"))

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
	for i := 0; i < 100; i++ {
		go func() {
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
			SetLevel(LogLevel(rand.Intn(len(levelStr))))
			Log(ERROR, "hello world")
		}()
	}
	SetLevel(DEBUG)
	assert.Equal(t, Level(), DEBUG)
}
