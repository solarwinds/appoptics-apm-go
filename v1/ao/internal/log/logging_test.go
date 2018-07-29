// Copyright (C) 2017 Librato, Inc. All rights reserved.

package log

import (
	"bytes"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/pkg/errors"
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
		{"APPOPTICS_DEBUG_LEVEL", "HelloWorld", defaultLogLevel},
		{"APPOPTICS_DEBUG_LEVEL", "0", DEBUG},
		{"APPOPTICS_DEBUG_LEVEL", "1", INFO},
		{"APPOPTICS_DEBUG_LEVEL", "2", WARNING},
		{"APPOPTICS_DEBUG_LEVEL", "3", ERROR},
		{"APPOPTICS_DEBUG_LEVEL", "4", defaultLogLevel},
		{"APPOPTICS_DEBUG_LEVEL", "1000", defaultLogLevel},
	}

	for _, test := range tests {
		os.Setenv(test.key, test.val)
		initLog()
		assert.EqualValues(t, test.expected, Level(), "Test-"+test.val)
	}

	os.Unsetenv("APPOPTICS_DEBUG_LEVEL")
	initLog()
	assert.EqualValues(t, Level(), defaultLogLevel)
}

func TestLog(t *testing.T) {
	var buffer bytes.Buffer
	log.SetOutput(&buffer)

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	initLog()

	tests := map[string]string{
		"hello world": "hello world\n",
		"":            "\n",
		"hello %s":    "hello %!s(MISSING)\n",
	}

	for str, expected := range tests {
		buffer.Reset()
		Logf(INFO, str)
		assert.True(t, strings.HasSuffix(buffer.String(), expected))
	}

	buffer.Reset()
	Log(INFO, 1, 2, 3)
	assert.True(t, strings.HasSuffix(buffer.String(), "1 2 3\n"))

	buffer.Reset()
	Debug(1, "abc", 3)
	assert.True(t, strings.HasSuffix(buffer.String(), "1abc3\n"))

	buffer.Reset()
	Error(errors.New("hello"))
	assert.True(t, strings.HasSuffix(buffer.String(), "hello\n"))

	buffer.Reset()
	Warning("Áú")
	assert.True(t, strings.HasSuffix(buffer.String(), "Áú\n"))

	buffer.Reset()
	Info("hello")
	assert.True(t, strings.HasSuffix(buffer.String(), "\n"))

	buffer.Reset()
	Warningf("hello %s", "world")
	assert.True(t, strings.HasSuffix(buffer.String(), "hello world\n"))

	buffer.Reset()
	Infof("show me the %v", "code")
	assert.True(t, strings.HasSuffix(buffer.String(), "show me the code\n"))

	log.SetOutput(os.Stderr)
	os.Unsetenv("APPOPTICS_DEBUG_LEVEL")

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
		"ABC":     defaultLogLevel,
	}
	for str, lvl := range tests {
		l, _ := ToLogLevel(str)
		assert.Equal(t, lvl, l)
	}
}

func TestSetLevel(t *testing.T) {
	var buf utils.SafeBuffer
	var writers []io.Writer

	writers = append(writers, &buf)
	writers = append(writers, os.Stderr)

	log.SetOutput(io.MultiWriter(writers...))

	defer func() {
		log.SetOutput(os.Stderr)
	}()

	SetLevel(INFO)
	var wg = &sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(wg *sync.WaitGroup) {
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
			Debug("hello world")
			wg.Done()
		}(wg)
	}
	wg.Wait()
	assert.Equal(t, "", buf.String())

	buf.Reset()
	SetLevel(DEBUG)
	Debug("test")
	assert.True(t, strings.Contains(buf.String(), "test"))
	buf.Reset()
	Error("", "one", "two", "three")
	assert.Equal(t, DEBUG, Level())
	assert.True(t, strings.Contains(buf.String(), "onetwothree"))
}
