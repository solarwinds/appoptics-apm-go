package agent

import (
	"testing"

	"os"

	"github.com/stretchr/testify/assert"
)

func TestGetConfig(t *testing.T) {
	agentConf.initialized = false
	assert.Equal(t, "", GetConfig(ConfName("APPOPTICS_COLLECTOR")))

	agentConf.initialized = true
	assert.Equal(t, "", GetConfig(ConfName("INVALID")))

	assert.Equal(t, "collector.appoptics.com:443", GetConfig(ConfName("APPOPTICS_COLLECTOR")))
	assert.Equal(t, "", GetConfig(ConfName("")))

	os.Setenv("APPOPTICS_COLLECTOR", "test.com:12345")
	Init()
	assert.Equal(t, "test.com:12345", GetConfig(ConfName("APPOPTICS_COLLECTOR")))
	os.Unsetenv("APPOPTICS_COLLECTOR")
	Init()
	assert.Equal(t, "collector.appoptics.com:443", GetConfig(ConfName("APPOPTICS_COLLECTOR")))
}
