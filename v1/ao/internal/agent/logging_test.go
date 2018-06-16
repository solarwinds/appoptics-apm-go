package agent

import (
	"os"
	"testing"

	"log"

	"bytes"

	"strings"

	"fmt"

	"github.com/stretchr/testify/assert"
)

func TestDebugLevel(t *testing.T) {
	os.Setenv("APPOPTICS_DEBUG_LEVEL", "DEBUG")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(0))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "Info")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(1))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "warn")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(2))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "erroR")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", " erroR  ")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "HelloWorld")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "0")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(0))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "1")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(1))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "2")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(2))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "3")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "4")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(3))

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "1000")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(3))

	os.Unsetenv("APPOPTICS_DEBUG_LEVEL")
	Init()
	assert.EqualValues(t, debugLevel, DebugLevel(2))
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
	fmt.Println("----" + buffer.String() + "----")
	assert.True(t, strings.HasSuffix(buffer.String(), "hello %!s(<nil>)"+"\n"))

}
