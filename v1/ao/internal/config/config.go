// Copyright (C) 2017 Librato, Inc. All rights reserved.

// Package config is responsible for loading the configuration from various
// sources, e.g., environment variables, configuration files and user input.
// It also accepts dynamic settings from the collector server.
package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// The default values for environment variables
const (
	defaultGRPCCollector      = "collector.appoptics.com:443"
	defaultServiceKey         = ""
	defaultTrustedPath        = ""
	defaultCollectorUDP       = "127.0.0.1:7831"
	defaultReporter           = "ssl"
	defaultTracingMode        = "enabled"
	maxSampleRate             = 1000000
	defaultSampleRate         = maxSampleRate
	defaultPrependDomain      = false
	defaultHostnameAlias      = ""
	defaultInsecureSkipVerify = false
	defaultHistogramPrecision = 2
	defaultDisabled           = false
)

// The environment variables
const (
	envAppOpticsCollector           = "APPOPTICS_COLLECTOR"
	envAppOpticsServiceKey          = "APPOPTICS_SERVICE_KEY"
	envAppOpticsTrustedPath         = "APPOPTICS_TRUSTEDPATH"
	envAppOpticsCollectorUDP        = "APPOPTICS_COLLECTOR_UDP"
	envAppOpticsReporter            = "APPOPTICS_REPORTER"
	envAppOpticsTracingMode         = "APPOPTICS_TRACING_MODE"
	envAppOpticsSampleRate          = "APPOPTICS_SAMPLE_RATE"
	envAppOpticsPrependDomain       = "APPOPTICS_PREPEND_DOMAIN"
	envAppOpticsHostnameAlias       = "APPOPTICS_HOSTNAME_ALIAS"
	envAppOpticsInsecureSkipVerify  = "APPOPTICS_INSECURE_SKIP_VERIFY"
	envAppOpticsHistogramPrecision  = "APPOPTICS_HISTOGRAM_PRECISION"
	envAppOpticsEventsFlushInterval = "APPOPTICS_EVENTS_FLUSH_INTERVAL"
	envAppOpticsEventsBatchSize     = "APPOPTICS_EVENTS_BATCHSIZE"
	envAppOpticsDisabled            = "APPOPTICS_DISABLED"
)

// The configuration items
const (
	cnfCollector           = "Collector"
	cnfServiceKey          = "ServiceKey"
	cnfTrustedPath         = "TrustedPath"
	cnfCollectorUDP        = "CollectorUDP"
	cnfReporterType        = "ReporterType"
	cnfTracingMode         = "TracingMode"
	cnfSampleRate          = "SampleRate"
	cnfPrependDomain       = "PrependDomain"
	cnfHostAlias           = "HostAlias"
	cnfSkipVerify          = "SkipVerify"
	cnfPrecision           = "Precision"
	cnfEventsFlushInterval = "EventsFlushInterval"
	cnfEventsBatchSize     = "EventsBatchSize"
	cnfDisabled            = "Disabled"
)

// Errors
var (
	ErrUnsupportedFormat = errors.New("unsupported format")
	ErrFileTooLarge      = errors.New("file size exceeds limit")
)

// max config file size = 1MB
const maxConfigFileSize = 1024 * 1024

// The environment variables, validators and converters. This map is not
// concurrent-safe and should not be modified after initialization.
var envs = map[string]Env{
	cnfCollector: {
		name:     envAppOpticsCollector,
		optional: true,
		validate: IsValidHost,
		convert:  ToHost,
		mask:     nil,
	},
	cnfServiceKey: {
		name:     envAppOpticsServiceKey,
		optional: false,
		validate: IsValidServiceKey,
		convert:  ToServiceKey,
		mask:     MaskServiceKey,
	},
	cnfTrustedPath: {
		name:     envAppOpticsTrustedPath,
		optional: true,
		validate: IsValidFileString,
		convert:  ToFileString,
		mask:     nil,
	},
	cnfCollectorUDP: {
		name:     envAppOpticsCollectorUDP,
		optional: true,
		validate: IsValidHost,
		convert:  ToHost,
		mask:     nil,
	},
	cnfReporterType: {
		name:     envAppOpticsReporter,
		optional: true,
		validate: IsValidReporterType,
		convert:  ToReporterType,
		mask:     nil,
	},
	cnfTracingMode: {
		name:     envAppOpticsTracingMode,
		optional: true,
		validate: IsValidTracingMode,
		convert:  ToTracingMode,
		mask:     nil,
	},
	cnfSampleRate: {
		name:     envAppOpticsSampleRate,
		optional: true,
		validate: IsValidSampleRate,
		convert:  ToInteger,
		mask:     nil,
	},
	cnfPrependDomain: {
		name:     envAppOpticsPrependDomain,
		optional: true,
		validate: IsValidBool,
		convert:  ToBool,
		mask:     nil,
	},
	cnfHostAlias: {
		name:     envAppOpticsHostnameAlias,
		optional: true,
		validate: IsValidHostnameAlias,
		convert:  ToHostnameAlias,
		mask:     nil,
	},
	cnfSkipVerify: {
		name:     envAppOpticsInsecureSkipVerify,
		optional: true,
		validate: IsValidBool,
		convert:  ToBool,
		mask:     nil,
	},
	cnfPrecision: {
		name:     envAppOpticsHistogramPrecision,
		optional: true,
		validate: IsValidInteger,
		convert:  ToInteger,
		mask:     nil,
	},
	cnfEventsFlushInterval: {
		name:     envAppOpticsEventsFlushInterval,
		optional: true,
		validate: IsValidInteger,
		convert:  ToInt64,
		mask:     nil,
	},
	cnfEventsBatchSize: {
		name:     envAppOpticsEventsBatchSize,
		optional: true,
		validate: IsValidInteger,
		convert:  ToInt64,
		mask:     nil,
	},
	cnfDisabled: {
		name:     envAppOpticsDisabled,
		optional: true,
		validate: IsValidBool,
		convert:  ToBool,
		mask:     nil,
	},
}

// Config is the struct to define the agent configuration. The configuration
// options in this struct (excluding those from ReporterOptions) are not
// intended for dynamically updating.
type Config struct {
	sync.RWMutex

	// Collector defines the host and port of the AppOptics collector
	Collector string `yaml:"CollectorHost,omitempty" json:"CollectorHost,omitempty"`

	// ServiceKey defines the service key and service name
	ServiceKey string `yaml:"ServiceKey,omitempty" json:"ServiceKey,omitempty"`

	// The file path of the cert file for gRPC connection
	TrustedPath string `yaml:"TrustedPath,omitempty" json:"TrustedPath,omitempty"`

	// The host and port of the UDP collector
	CollectorUDP string `yaml:"CollectorHostUDP,omitempty" json:"CollectorHostUDP,omitempty"`

	// The reporter type, ssl or udp
	ReporterType string `yaml:"ReporterType,omitempty" json:"ReporterType,omitempty"`

	// The tracing mode
	TracingMode string `yaml:"TracingMode,omitempty" json:"TracingMode,omitempty"`

	// The sample rate
	SampleRate int `yaml:"SampleRate,omitempty" json:"SampleRate,omitempty"`

	// Either local tracing mode or sampling rate is configured
	samplingConfigured bool

	// Whether the domain should be prepended to the transaction name.
	PrependDomain bool `yaml:"PrependDomain,omitempty" json:"PrependDomain,omitempty"`

	// The alias of the hostname
	HostAlias string `yaml:"HostnameAlias,omitempty" json:"HostnameAlias,omitempty"`

	// Whether to skip verification of hostname
	SkipVerify bool `yaml:"InsecureSkipVerify,omitempty" json:"InsecureSkipVerify,omitempty"`

	// The precision of the histogram
	Precision int `yaml:"HistogramPrecision,omitempty" json:"HistogramPrecision,omitempty"`

	// The reporter options
	Reporter *ReporterOptions `yaml:"ReporterOptions,omitempty" json:"ReporterOptions,omitempty"`

	Disabled bool `yaml:"Disabled,omitempty" json:"Disabled,omitempty"`
}

// Option is a function type that accepts a Config pointer and
// applies the configuration option it defines.
type Option func(c *Config)

// WithCollector defines a Config option for collector address.
func WithCollector(collector string) Option {
	return func(c *Config) {
		c.Collector = collector
	}
}

// WithServiceKey defines a Config option for the service key.
func WithServiceKey(key string) Option {
	return func(c *Config) {
		c.ServiceKey = key
	}
}

// NewConfig initializes a ReporterOptions object and override default values
// with options provided as arguments. It may print errors if there are invalid
// values in the configuration file or the environment variables.
//
// If there is an error (e.g., invalid config option value), it will return a
// config with default values and DISABLE the agent.
func NewConfig(opts ...Option) *Config {
	c := newConfig()
	if err := c.RefreshConfig(opts...); err != nil {
		log.Errorf("Agent disabled due to config init failure: %s", err.Error())
		c.reset()
		c.setDisabled(true)
	}
	return c
}

func (c *Config) validate() error {
	// TODO
	return nil
}

// RefreshConfig loads the customized settings and merge with default values
func (c *Config) RefreshConfig(opts ...Option) error {
	c.Lock()
	defer c.Unlock()

	c.reset()

	if err := c.loadConfigFile(); err != nil {
		return errors.Wrap(err, "RefreshConfig")
	}
	c.loadEnvs()

	for _, opt := range opts {
		opt(c)
	}
	return c.validate()
}

func newConfig() *Config {
	return &Config{}
}

func (c *Config) reset() {
	c.Collector = defaultGRPCCollector
	c.ServiceKey = defaultServiceKey
	c.TrustedPath = defaultTrustedPath
	c.CollectorUDP = defaultCollectorUDP
	c.ReporterType = defaultReporter
	c.TracingMode = defaultTracingMode
	c.SampleRate = defaultSampleRate
	c.samplingConfigured = false
	c.PrependDomain = defaultPrependDomain
	c.HostAlias = defaultHostnameAlias
	c.SkipVerify = defaultInsecureSkipVerify
	c.Precision = defaultHistogramPrecision
	c.Reporter = defaultReporterOptions()
	c.Disabled = defaultDisabled
}

// loadEnvs loads environment variable values and update the Config object.
func (c *Config) loadEnvs() {
	c.Collector = envs[cnfCollector].LoadString(c.Collector)
	c.ServiceKey = envs[cnfServiceKey].LoadString(c.ServiceKey)

	c.TrustedPath = envs[cnfTrustedPath].LoadString(c.TrustedPath)
	c.CollectorUDP = envs[cnfCollectorUDP].LoadString(c.CollectorUDP)
	c.ReporterType = envs[cnfReporterType].LoadString(c.ReporterType)

	tracingMode := envs[cnfTracingMode].LoadString("INVALID")
	if tracingMode != "INVALID" {
		c.TracingMode = tracingMode
		c.samplingConfigured = true
	}
	sampleRate := envs[cnfSampleRate].LoadInt(maxSampleRate + 1)
	if sampleRate != maxSampleRate+1 {
		c.SampleRate = sampleRate
		c.samplingConfigured = true
	}

	c.PrependDomain = envs[cnfPrependDomain].LoadBool(c.PrependDomain)
	c.HostAlias = envs[cnfHostAlias].LoadString(c.HostAlias)
	c.SkipVerify = envs[cnfSkipVerify].LoadBool(c.SkipVerify)

	c.Precision = envs[cnfPrecision].LoadInt(c.Precision)
	c.Disabled = envs[cnfDisabled].LoadBool(c.Disabled)

	c.Reporter.loadEnvs()
}

func (c *Config) getConfigPath() string {
	path, ok := os.LookupEnv("APPOPTICS_CONFIG_FILE")
	if ok {
		return path
	}

	candidates := []string{
		"./appoptics-goagent.yaml",
		"./appoptics-goagent.yml",
		"/appoptics-goagent.yaml",
		"/appoptics-goagent.yml",
	}

	for _, file := range candidates {
		abs, err := filepath.Abs(file)
		if err != nil {
			continue
		}
		if _, e := os.Stat(abs); e != nil {
			continue
		}
		return abs
	}

	return ""
}

func (c *Config) loadYaml(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrap(err, "loadYaml")
	}

	// The config struct is modified in place so we won't tolerate any error
	err = yaml.Unmarshal(data, &c)
	if err != nil {
		return errors.Wrap(err, "loadYaml")
	}
	return nil
}

func (c *Config) checkFileSize(path string) error {
	file, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "checkFileSize")
	}
	size := file.Size()
	if size > maxConfigFileSize {
		return errors.Wrap(ErrFileTooLarge, fmt.Sprintf("File size: %d", size))
	}
	return nil
}

// loadConfigFile loads from the config file
func (c *Config) loadConfigFile() error {
	path := c.getConfigPath()
	if path == "" {
		log.Info("No config file found.")
		return nil
	}

	if err := c.checkFileSize(path); err != nil {
		return errors.Wrap(err, "loadConfigFile")
	}
	ext := filepath.Ext(path)

	switch ext {
	case ".yml", ".yaml":
		log.Warningf("Loading config file: %s", path)
		return c.loadYaml(path)
	default:
		return errors.Wrap(ErrUnsupportedFormat, fmt.Sprintf("loadConfigFile: %s", path))
	}
}

// GetCollector returns the collector address
func (c *Config) GetCollector() string {
	c.RLock()
	defer c.RUnlock()
	return c.Collector
}

// GetServiceKey returns the service key
func (c *Config) GetServiceKey() string {
	c.RLock()
	defer c.RUnlock()
	return c.ServiceKey
}

// GetTrustedPath returns the file path of the cert file
func (c *Config) GetTrustedPath() string {
	c.RLock()
	defer c.RUnlock()
	return c.TrustedPath
}

// GetReporterType returns the reporter type
func (c *Config) GetReporterType() string {
	c.RLock()
	defer c.RUnlock()
	return c.ReporterType
}

// GetCollectorUDP returns the UDP collector host
func (c *Config) GetCollectorUDP() string {
	c.RLock()
	defer c.RUnlock()
	return c.CollectorUDP
}

// GetTracingMode returns the local tracing mode
func (c *Config) GetTracingMode() string {
	c.RLock()
	defer c.RUnlock()
	return c.TracingMode
}

// GetSampleRate returns the local sample rate
func (c *Config) GetSampleRate() int {
	c.RLock()
	defer c.RUnlock()
	return c.SampleRate
}

// HasLocalSamplingConfig returns if local tracing mode or sampling rate is configured
func (c *Config) HasLocalSamplingConfig() bool {
	c.RLock()
	defer c.RUnlock()
	return c.samplingConfigured
}

// GetPrependDomain returns the prepend domain config
func (c *Config) GetPrependDomain() bool {
	c.RLock()
	defer c.RUnlock()
	return c.PrependDomain
}

// GetHostAlias returns the host alias
func (c *Config) GetHostAlias() string {
	c.RLock()
	defer c.RUnlock()
	return c.HostAlias
}

// GetSkipVerify returns the config of skipping hostname verification
func (c *Config) GetSkipVerify() bool {
	c.RLock()
	defer c.RUnlock()
	return c.SkipVerify
}

// GetPrecision returns the histogram precision
func (c *Config) GetPrecision() int {
	c.RLock()
	defer c.RUnlock()
	return c.Precision
}

// GetDisabled returns if the agent is disabled
func (c *Config) GetDisabled() bool {
	c.RLock()
	defer c.RUnlock()
	return c.Disabled
}

func (c *Config) setDisabled(disabled bool) {
	c.RLock()
	defer c.RUnlock()
	c.Disabled = disabled
}

// GetReporter returns the reporter options struct
func (c *Config) GetReporter() *ReporterOptions {
	c.RLock()
	defer c.RUnlock()
	return c.Reporter
}
