package agent

import (
	"bytes"
	"log"
	"os"
	"testing"

	"strings"

	"github.com/stretchr/testify/assert"
)

func TestInitConf(t *testing.T) {
	var buffer bytes.Buffer

	os.Setenv("APPOPTICS_DEBUG_LEVEL", "debug")
	log.SetOutput(&buffer)
	Init()
	assert.True(t, strings.HasSuffix(buffer.String(), "non-default configuration used APPOPTICS_DEBUG_LEVEL=debug\n"))
}
