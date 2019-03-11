// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	key1 := "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"
	key2 := "bbbb315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"

	os.Setenv(envAppOpticsCollector, "example.com:12345")
	os.Setenv(envAppOpticsPrependDomain, "true")
	os.Setenv(envAppOpticsHistogramPrecision, "2")
	os.Setenv(envAppOpticsServiceKey, key1)
	os.Setenv(envAppOpticsDisabled, "true")

	c := NewConfig()
	assert.Equal(t, "example.com:12345", c.GetCollector())
	assert.Equal(t, true, c.PrependDomain)
	assert.Equal(t, 2, c.Precision)
	assert.Equal(t, true, c.Disabled)

	os.Setenv(envAppOpticsCollector, "test.abc:8080")
	os.Setenv(envAppOpticsDisabled, "false")
	os.Setenv(envAppOpticsTracingMode, "always")

	c.RefreshConfig()
	assert.Equal(t, "test.abc:8080", c.GetCollector())
	assert.Equal(t, false, c.Disabled)
	assert.Equal(t, "enabled", c.GetTracingMode())

	c = NewConfig(
		WithCollector("hello.world"),
		WithServiceKey(key2))
	assert.Equal(t, "hello.world", c.GetCollector())
	assert.Equal(t, ToServiceKey(key2), c.GetServiceKey())

	os.Setenv(envAppOpticsServiceKey, key1)
	os.Setenv(envAppOpticsHostnameAlias, "test")
	os.Setenv(envAppOpticsInsecureSkipVerify, "false")
	os.Setenv(envAppOpticsTrustedPath, "test.crt")
	os.Setenv(envAppOpticsCollectorUDP, "hello.udp")
	os.Setenv(envAppOpticsDisabled, "invalidValue")

	c.RefreshConfig()
	assert.Equal(t, ToServiceKey(key1), c.GetServiceKey())
	assert.Equal(t, "test", c.GetHostAlias())
	assert.Equal(t, false, c.GetSkipVerify())
	assert.Equal(t, "test.crt", filepath.Base(c.GetTrustedPath()))
	assert.Equal(t, "hello.udp", c.GetCollectorUDP())
	assert.Equal(t, false, c.GetDisabled())
}

func TestConfig_HasLocalSamplingConfig(t *testing.T) {
	// Set tracing mode
	_ = os.Setenv(envAppOpticsTracingMode, "disabled")
	Refresh()
	assert.True(t, SamplingConfigured())
	assert.Equal(t, "disabled", GetTracingMode())
	assert.Equal(t, ToInteger(getFieldDefaultValue(&SamplingConfig{}, "SampleRate")), GetSampleRate())

	// No local sampling config
	_ = os.Unsetenv(envAppOpticsTracingMode)
	Refresh()
	assert.False(t, SamplingConfigured())
	assert.Equal(t, getFieldDefaultValue(&SamplingConfig{}, "TracingMode"), GetTracingMode())
	assert.Equal(t, ToInteger(getFieldDefaultValue(&SamplingConfig{}, "SampleRate")), GetSampleRate())

	// Set sample rate to 10000
	_ = os.Setenv(envAppOpticsSampleRate, "10000")
	Refresh()
	assert.True(t, SamplingConfigured())
	assert.Equal(t, getFieldDefaultValue(&SamplingConfig{}, "TracingMode"), GetTracingMode())
	assert.Equal(t, 10000, GetSampleRate())
}

func TestPrintDelta(t *testing.T) {
	changed := newConfig().reset()
	changed.Collector = "test.com:443"
	changed.PrependDomain = true
	changed.ReporterProperties.EventFlushInterval = 100

	assert.Equal(t, "Collector(APPOPTICS_COLLECTOR)=test.com:443 (default=collector.appoptics.com:443)\nPrependDomain(APPOPTICS_PREPEND_DOMAIN)=true (default=false)",
		getDelta(newConfig().reset(), changed).sanitize().String())
}

func TestConfigInit(t *testing.T) {
	c := newConfig()

	// Set them to true, the call to `reset` in next step should reset them to false
	c.Sampling.sampleRateConfigured = true
	c.Sampling.tracingModeConfigured = true

	c.reset()

	defaultC := Config{
		Collector:    "collector.appoptics.com:443",
		ServiceKey:   "",
		TrustedPath:  "",
		CollectorUDP: "",
		ReporterType: "ssl",
		Sampling: &SamplingConfig{
			TracingMode:           "enabled",
			tracingModeConfigured: false,
			SampleRate:            1000000,
			sampleRateConfigured:  false,
		},
		PrependDomain: false,
		HostAlias:     "",
		SkipVerify:    false,
		Precision:     2,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2,
			EventFlushBatchSize:     2000,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		Disabled: false,
	}
	assert.Equal(t, *c, defaultC)
}

func ClearEnvs() {
	for _, kv := range os.Environ() {
		kvSlice := strings.Split(kv, "=")
		k := kvSlice[0]
		os.Unsetenv(k)
	}
}

func SetEnvs(kvs []string) {
	for _, kv := range kvs {
		kvSlice := strings.Split(kv, "=")
		k, v := kvSlice[0], kvSlice[1]
		os.Setenv(k, v)
	}
}

func TestEnvsLoading(t *testing.T) {
	ClearEnvs()

	envs := []string{
		"APPOPTICS_COLLECTOR=collector.test.com",
		"APPOPTICS_SERVICE_KEY=ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		"APPOPTICS_TRUSTEDPATH=/collector.crt",
		"APPOPTICS_COLLECTOR_UDP=udp.test.com",
		"APPOPTICS_REPORTER=udp",
		"APPOPTICS_TRACING_MODE=never",
		"APPOPTICS_SAMPLE_RATE=1000",
		"APPOPTICS_PREPEND_DOMAIN=true",
		"APPOPTICS_HOSTNAME_ALIAS=alias",
		"APPOPTICS_INSECURE_SKIP_VERIFY=true",
		"APPOPTICS_HISTOGRAM_PRECISION=4",
		"APPOPTICS_EVENTS_FLUSH_INTERVAL=4",
		"APPOPTICS_EVENTS_BATCHSIZE=4000",
		"APPOPTICS_DISABLED=true",
	}
	SetEnvs(envs)

	envConfig := Config{
		Collector:    "collector.test.com",
		ServiceKey:   "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		TrustedPath:  "/collector.crt",
		CollectorUDP: "udp.test.com",
		ReporterType: "udp",
		Sampling: &SamplingConfig{
			TracingMode:           "disabled",
			tracingModeConfigured: true,
			SampleRate:            1000,
			sampleRateConfigured:  true,
		},
		PrependDomain: true,
		HostAlias:     "alias",
		SkipVerify:    true,
		Precision:     2 * 2,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2 * 2,
			EventFlushBatchSize:     2000 * 2,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		Disabled: true,
	}

	c := NewConfig()

	assert.Equal(t, *c, envConfig)
}
