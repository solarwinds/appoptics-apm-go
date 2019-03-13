// Copyright (C) 2017 Librato, Inc. All rights reserved.

// Package config is responsible for loading the configuration from various
// sources, e.g., environment variables, configuration files and user input.
// It also accepts dynamic settings from the collector server.
//
// In order to add a new configuration item, you need to:
// - add a field to the Config struct and assign the corresponding env variable
//   name and the default value via struct tags.
// - add validation code to method `Config.validate()` (optional).
// - add a method to retrieve the config value and a wrapper for the default
//   global variable `conf` (see wrappers.go).
package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	// MaxSampleRate is the maximum sample rate we can have
	MaxSampleRate = 1000000
	// MinSampleRate is the minimum sample rate we can have
	MinSampleRate = 0
	// max config file size = 1MB
	maxConfigFileSize = 1024 * 1024
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
	envAppOpticsConfigFile          = "APPOPTICS_CONFIG_FILE"
)

// Errors
var (
	ErrUnsupportedFormat = errors.New("unsupported format")
	ErrFileTooLarge      = errors.New("file size exceeds limit")
	ErrInvalidServiceKey = errors.New("invalid service key")
)

// Config is the struct to define the agent configuration. The configuration
// options in this struct (excluding those from ReporterOptions) are not
// intended for dynamically updating.
type Config struct {
	sync.RWMutex `yaml:"-"`

	// Collector defines the host and port of the AppOptics collector
	Collector string `yaml:",omitempty" env:"APPOPTICS_COLLECTOR" default:"collector.appoptics.com:443"`

	// ServiceKey defines the service key and service name
	ServiceKey string `yaml:",omitempty" env:"APPOPTICS_SERVICE_KEY"`

	// The file path of the cert file for gRPC connection
	TrustedPath string `yaml:",omitempty" env:"APPOPTICS_TRUSTEDPATH"`

	// The host and port of the UDP collector
	CollectorUDP string `yaml:",omitempty" env:"APPOPTICS_COLLECTOR_UDP"`

	// The reporter type, ssl or udp
	ReporterType string `yaml:",omitempty" env:"APPOPTICS_REPORTER" default:"ssl"`

	Sampling *SamplingConfig `yaml:",omitempty"`

	// Whether the domain should be prepended to the transaction name.
	PrependDomain bool `yaml:",omitempty" env:"APPOPTICS_PREPEND_DOMAIN"`

	// The alias of the hostname
	HostAlias string `yaml:",omitempty" env:"APPOPTICS_HOSTNAME_ALIAS"`

	// Whether to skip verification of hostname
	SkipVerify bool `yaml:",omitempty" env:"APPOPTICS_INSECURE_SKIP_VERIFY"`

	// The precision of the histogram
	Precision int `yaml:",omitempty" env:"APPOPTICS_HISTOGRAM_PRECISION" default:"2"`

	// The reporter options
	ReporterProperties *ReporterOptions `yaml:",omitempty"`

	Disabled bool `yaml:",omitempty" env:"APPOPTICS_DISABLED"`

	DebugLevel string `yaml:",omitempty" env:"APPOPTICS_DEBUG_LEVEL"`
}

// SamplingConfig defines the configuration options for the sampling decision
type SamplingConfig struct {
	// The tracing mode
	TracingMode string `yaml:",omitempty" env:"APPOPTICS_TRACING_MODE" default:"enabled"`
	// If the tracing mode is configured explicitly
	tracingModeConfigured bool `yaml:"-"`

	// The sample rate
	SampleRate int `yaml:",omitempty" env:"APPOPTICS_SAMPLE_RATE" default:"1000000"`
	// If the sample rate is configured explicitly
	sampleRateConfigured bool `yaml:"-"`
}

// Configured returns if either the tracing mode or the sampling rate has been configured
func (s *SamplingConfig) Configured() bool {
	return s.tracingModeConfigured || s.sampleRateConfigured
}

// UnmarshalYAML is the customized unmarshal method for SamplingConfig
func (s *SamplingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var aux = struct {
		TracingMode string
		SampleRate  int
	}{
		TracingMode: "Invalid",
		SampleRate:  -1,
	}
	if err := unmarshal(&aux); err != nil {
		return errors.Wrap(err, "failed to unmarshal SamplingConfig")
	}

	if aux.TracingMode != "Invalid" {
		s.SetTracingMode(aux.TracingMode)
	}
	if aux.SampleRate != -1 {
		s.SetSampleRate(aux.SampleRate)
	}
	return nil
}

// ResetTracingMode resets the tracing mode to its default value and clear the flag.
func (s *SamplingConfig) ResetTracingMode() {
	s.TracingMode = getFieldDefaultValue(s, "TracingMode")
	s.tracingModeConfigured = false
}

// ResetSampleRate resets the sample rate to its default value and clear the flag.
func (s *SamplingConfig) ResetSampleRate() {
	s.SampleRate = ToInteger(getFieldDefaultValue(s, "SampleRate"))
	s.sampleRateConfigured = false
}

func (s *SamplingConfig) validate() {
	if ok := IsValidTracingMode(s.TracingMode); !ok {
		log.Warning(InvalidEnv("TracingMode", s.TracingMode))
		s.ResetTracingMode()
	}
	if ok := IsValidSampleRate(s.SampleRate); !ok {
		log.Warning(InvalidEnv("SampleRate", strconv.Itoa(s.SampleRate)))
		s.ResetSampleRate()
	}
}

// SetTracingMode assigns the tracing mode and set the corresponding flag.
// Note: Do not change the method name as it (`Set`+Field name) is used in method
// `loadEnvsInternal` to assign the values loaded from env variables dynamically.
func (s *SamplingConfig) SetTracingMode(mode string) {
	s.TracingMode = ToTracingMode(mode)
	s.tracingModeConfigured = true
}

// SetSampleRate assigns the sample rate and set the corresponding flag.
// Note: Do not change the method name as it (`Set`+Field name) is used in method
// `loadEnvsInternal` to assign the values loaded from env variables dynamically.
func (s *SamplingConfig) SetSampleRate(rate int) {
	s.SampleRate = rate
	s.sampleRateConfigured = true
}

// Get the value of the `default` tag of a field in the struct.
func getFieldDefaultValue(i interface{}, fieldName string) string {
	iv := reflect.Indirect(reflect.ValueOf(i))
	if iv.Kind() != reflect.Struct {
		panic("calling getFieldDefaultValue with non-struct type")
	}

	field, ok := iv.Type().FieldByName(fieldName)
	if !ok {
		panic(fmt.Sprintf("invalid field: %s", fieldName))
	}

	return field.Tag.Get("default")
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

// NewConfig initializes a Config object and override default values with options
// provided as arguments. It may print errors if there are invalid values in the
// configuration file or the environment variables.
//
// If there is a fatal error (e.g., invalid config file), it will return a config
// object with default values.
func NewConfig(opts ...Option) *Config {
	c := newConfig()
	if err := c.Load(opts...); err != nil {
		log.Error(errors.Wrap(err, "Failed to initialize configuration"))
		c.reset()
	}
	return c
}

func (c *Config) validate() error {
	if ok := IsValidHost(c.Collector); !ok {
		log.Warning(InvalidEnv("Collector", c.Collector))
		c.Collector = getFieldDefaultValue(c, "Collector")
	}

	c.ServiceKey = ToServiceKey(c.ServiceKey)
	if ok := IsValidServiceKey(c.ServiceKey); !ok {
		log.Warning(MissingEnv("ServiceKey"))
		return errors.Wrap(ErrInvalidServiceKey, fmt.Sprintf("\"%s\"", c.ServiceKey))
	}

	if ok := IsValidFile(c.TrustedPath); !ok {
		log.Warning(InvalidEnv("TrustedPath", c.TrustedPath))
		c.TrustedPath = getFieldDefaultValue(c, "TrustedPath")
	}

	if ok := IsValidHost(c.CollectorUDP); !ok {
		log.Warning(InvalidEnv("CollectorUDP", c.CollectorUDP))
		c.CollectorUDP = getFieldDefaultValue(c, "CollectorUDP")
	}

	c.ReporterType = strings.ToLower(strings.TrimSpace(c.ReporterType))
	if ok := IsValidReporterType(c.ReporterType); !ok {
		log.Warning(InvalidEnv("ReporterType", c.ReporterType))
		c.ReporterType = getFieldDefaultValue(c, "ReporterType")
	}

	c.Sampling.validate()

	if ok := IsValidHostnameAlias(c.HostAlias); !ok {
		log.Warning(InvalidEnv("HostAlias", c.HostAlias))
		c.HostAlias = getFieldDefaultValue(c, "HostAlias")
	}

	if _, valid := log.ToLogLevel(c.DebugLevel); !valid {
		log.Warning(InvalidEnv("DebugLevel", c.DebugLevel))
		c.DebugLevel = ""
	}

	return c.ReporterProperties.validate()
}

// Load reads configuration from config file and environment variables.
func (c *Config) Load(opts ...Option) error {
	c.Lock()
	defer c.Unlock()

	c.reset()

	if err := c.loadConfigFile(); err != nil {
		return errors.Wrap(err, "Load")
	}
	c.loadEnvs()

	for _, opt := range opts {
		opt(c)
	}
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "Load")
	}

	c.printDelta()

	return nil
}

func (c *Config) printDelta() {
	base := newConfig().reset()
	log.Warningf("Accepted config items: \n%s", getDelta(base, c, "").sanitize())
}

// DeltaItem defines a delta item  of two Config objects
type DeltaItem struct {
	key        string
	env        string
	value      string
	defaultVal string
}

// Delta defines the overall delta of two Config objects
type Delta struct {
	delta []DeltaItem
}

func (d *Delta) add(item ...DeltaItem) {
	d.delta = append(d.delta, item...)
}

func (d *Delta) items() []DeltaItem {
	return d.delta
}

func (d *Delta) sanitize() *Delta {
	for idx, item := range d.delta {
		if item.key == "ServiceKey" {
			d.delta[idx].value = MaskServiceKey(d.delta[idx].value)
			break
		}
	}
	return d
}

func (d *Delta) String() string {
	var s []string
	for _, item := range d.delta {
		s = append(s, fmt.Sprintf(" - %s (%s) = %s (default: %s)",
			item.key,
			item.env,
			item.value,
			item.defaultVal))
	}
	return strings.Join(s, "\n")
}

// getDelta compares two instances of the same struct and returns the delta.
func getDelta(base, changed interface{}, keyPrefix string) *Delta {
	delta := &Delta{}

	baseVal := reflect.Indirect(reflect.ValueOf(base))
	changedVal := reflect.Indirect(reflect.ValueOf(changed))

	if changedVal.Kind() != reflect.Struct {
		return delta
	}

	for i := 0; i < changedVal.NumField(); i++ {
		typeFieldChanged := changedVal.Type().Field(i)
		if typeFieldChanged.Anonymous {
			continue
		}

		fieldChanged := reflect.Indirect(changedVal.Field(i))
		fieldBase := reflect.Indirect(baseVal.Field(i))

		if fieldChanged.Kind() == reflect.Struct {
			prefix := typeFieldChanged.Name
			baseField := baseVal.Field(i).Interface()
			changedField := changedVal.Field(i).Interface()

			subDelta := getDelta(baseField, changedField, prefix)
			delta.add(subDelta.items()...)
		} else {
			if fieldChanged.CanSet() && // only collect the exported fields
				fieldBase.Interface() != fieldChanged.Interface() {
				keyName := typeFieldChanged.Name
				if keyPrefix != "" {
					keyName = fmt.Sprintf("%s.%s", keyPrefix, typeFieldChanged.Name)
				}
				kv := DeltaItem{
					key:        keyName,
					env:        typeFieldChanged.Tag.Get("env"),
					value:      fmt.Sprintf("%v", fieldChanged.Interface()),
					defaultVal: fmt.Sprintf("%v", fieldBase.Interface()),
				}
				delta.add(kv)
			}
		}
	}
	return delta
}

func newConfig() *Config {
	return &Config{
		Sampling:           &SamplingConfig{},
		ReporterProperties: &ReporterOptions{},
	}
}

func (c *Config) reset() *Config {
	return initStruct(c).(*Config)
}

// initStruct initializes the struct with the default values defined in the struct
// tags.
// The input must be a pointer to a settable struct object.
func initStruct(c interface{}) interface{} {
	cVal := reflect.Indirect(reflect.ValueOf(c))
	cType := cVal.Type()
	cPtrType := reflect.ValueOf(c).Type()

	for i := 0; i < cVal.NumField(); i++ {
		fieldVal := reflect.Indirect(cVal.Field(i))
		field := cType.Field(i)

		if field.Anonymous || !fieldVal.CanSet() {
			continue
		}
		if fieldVal.Kind() == reflect.Struct {
			// Need to use its pointer, otherwise it won't be addressable after
			// passed into the nested method
			initStruct(getValPtr(cVal.Field(i)).Interface())
		} else {
			tagVal := getFieldDefaultValue(c, field.Name)
			defaultVal := stringToValue(tagVal, field.Type)

			resetMethod := fmt.Sprintf("%s%s", "Reset", field.Name)
			var prefix string

			// The *T type is required to fetch the method defined with *T
			if _, ok := cPtrType.MethodByName(resetMethod); ok {
				prefix = "Reset"
			} else {
				prefix = "Set"
			}
			setField(c, prefix, field, defaultVal)
		}
	}

	return c
}

// setField assigns `val` to struct c's field specified by the name `field`. It
// first checks if there is a setter method `setterPrefix+FieldName` for this
// field, and calls the setter if so. Otherwise it sets the value directly via the
// reflect.Value.Set method.
//
// The setter, if provided, is preferred as there may be some extra work around,
// for example it may need to acquire a mutex, or set some flags.
// The `val` must have the same dynamic type as the field, otherwise it will panic.
//
// The dynamic type of the first argument `c` must be a pointer to a struct object.
func setField(c interface{}, prefix string, field reflect.StructField, val reflect.Value) {
	cVal := reflect.Indirect(reflect.ValueOf(c))
	if cVal.Kind() != reflect.Struct {
		return
	}

	fieldVal := reflect.Indirect(cVal.FieldByName(field.Name))
	if !fieldVal.IsValid() {
		return
	}

	fieldKind := field.Type.Kind()
	if !fieldVal.CanSet() || field.Anonymous || fieldKind == reflect.Struct {
		log.Warningf("Failed to set field: %s val: %v", field.Name, val.Interface())
		return
	}

	setMethodName := fmt.Sprintf("%s%s", prefix, field.Name)
	setMethodV := reflect.ValueOf(c).MethodByName(setMethodName)
	// setMethod may be invalid, but we call setMethodV.IsValid() first before
	// all the other methods so it should be safe.
	setMethodM, _ := reflect.TypeOf(c).MethodByName(setMethodName)
	setMethodT := setMethodM.Type

	// Call the setter if we have found a valid one
	if setMethodV.IsValid() {
		var in []reflect.Value
		// The method should not have more than 2 parameters, while the receiver
		// is the first parameter.
		if setMethodT.NumIn() == 2 && setMethodT.In(1).Kind() == fieldKind {
			in = append(in, val)
		}
		setMethodV.Call(in)
	} else {
		fieldVal.Set(val)
	}
}

// loadEnvs loads environment variable values and update the Config object.
func (c *Config) loadEnvs() {
	log.Warning("Loading environment variables.")
	loadEnvsInternal(c)
}

// getValPtr returns the pointer value of the input argument if it's not a Ptr
// The val must be addressable, otherwise it will panic.
func getValPtr(val reflect.Value) reflect.Value {
	if val.Kind() == reflect.Ptr {
		return val
	}
	return val.Addr()
}

// getConfigPath returns the absolute path of the config file.
func (c *Config) getConfigPath() string {
	path, ok := os.LookupEnv(envAppOpticsConfigFile)
	if ok {
		if abs, err := filepath.Abs(path); err == nil {
			return abs
		} else {
			log.Warningf("Ignore config file %s: %s", path, err)
		}
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
		return errors.Wrap(err, fmt.Sprintf("loadYaml: %s", path))
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

// loadConfigFile loads configuration from the config file.
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
		return errors.Wrap(ErrUnsupportedFormat, path)
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
	return c.Sampling.TracingMode
}

// GetSampleRate returns the local sample rate
func (c *Config) GetSampleRate() int {
	c.RLock()
	defer c.RUnlock()
	return c.Sampling.SampleRate
}

// SamplingConfigured returns if tracing mode or sampling rate is configured
func (c *Config) SamplingConfigured() bool {
	c.RLock()
	defer c.RUnlock()
	return c.Sampling.Configured()
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
	return c.ReporterProperties
}

// GetDebugLevel returns the global logging level. Note that it may return an
// empty string
func (c *Config) GetDebugLevel() string {
	c.RLock()
	defer c.RUnlock()
	return c.DebugLevel
}
