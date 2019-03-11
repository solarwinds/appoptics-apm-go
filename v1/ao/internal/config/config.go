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
}

// SamplingConfig defines the configuration options for the sampling decision
type SamplingConfig struct {
	// The tracing mode
	TracingMode string `yaml:",omitempty" env:"APPOPTICS_TRACING_MODE" default:"enabled"`
	// If the tracing mode is configured explicitly
	TracingModeConfigured bool `yaml:"-"`

	// The sample rate
	SampleRate int `yaml:",omitempty" env:"APPOPTICS_SAMPLE_RATE" default:"1000000"`
	// If the sample rate is configured explicitly
	SampleRateConfigured bool `yaml:"-"`
}

// Configured returns if either the tracing mode or the sampling rate has been configured
func (s *SamplingConfig) Configured() bool {
	return s.TracingModeConfigured || s.SampleRateConfigured
}

// UnmarshalYAML is the customized unmarshal method for SamplingConfig
func (s *SamplingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var aux = SamplingConfig{
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

func (s *SamplingConfig) resetTracingMode() {
	s.TracingMode = getFieldDefaultValue(s, "TracingMode")
	s.TracingModeConfigured = false
}

func (s *SamplingConfig) resetSampleRate() {
	s.SampleRate = ToInteger(getFieldDefaultValue(s, "SampleRate"))
	s.SampleRateConfigured = false
}

func (s *SamplingConfig) validate() {
	if ok := IsValidTracingMode(s.TracingMode); !ok {
		log.Warning(InvalidEnv("TracingMode", s.TracingMode))
		s.resetTracingMode()
	}
	if ok := IsValidSampleRate(s.SampleRate); !ok {
		log.Warning(InvalidEnv("SampleRate", strconv.Itoa(s.SampleRate)))
		s.resetSampleRate()
	}
}

// SetTracingMode assigns the tracing mode and set the corresponding flag.
// Note: Do not change the method name as it (`Set`+Field name) is used in method
// `loadEnvsInternal` to assign the values loaded from env variables dynamically.
func (s *SamplingConfig) SetTracingMode(mode string) {
	s.TracingMode = ToTracingMode(mode)
	s.TracingModeConfigured = true
}

// SetSampleRate assigns the sample rate and set the corresponding flag.
// Note: Do not change the method name as it (`Set`+Field name) is used in method
// `loadEnvsInternal` to assign the values loaded from env variables dynamically.
func (s *SamplingConfig) SetSampleRate(rate int) {
	s.SampleRate = rate
	s.SampleRateConfigured = true
}

func getFieldDefaultValue(i interface{}, name string) string {
	iv := reflect.Indirect(reflect.ValueOf(i))
	if iv.Kind() != reflect.Struct {
		panic("calling getFieldDefaultValue with non-struct type")
	}

	it := iv.Type()
	field, ok := it.FieldByName(name)
	if !ok {
		panic(fmt.Sprintf("invalid field: %s", name))
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

// NewConfig initializes a ReporterOptions object and override default values
// with options provided as arguments. It may print errors if there are invalid
// values in the configuration file or the environment variables.
//
// If there is an error (e.g., invalid config option value), it will return a
// config with default values and DISABLE the agent.
func NewConfig(opts ...Option) *Config {
	c := newConfig()
	if err := c.RefreshConfig(opts...); err != nil {
		e := errors.Wrap(err, "Config init failed, falling back to default values")
		log.Error(e)
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

	return c.ReporterProperties.validate()
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
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "RefreshConfig")
	}

	c.printDelta()

	return nil
}

func (c *Config) printDelta() {
	base := newConfig().reset()
	log.Warningf("Accepted config items: \n%s", getDelta(base, c).sanitize())
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
		s = append(s, fmt.Sprintf("%s(%s)=%s (default=%s)",
			item.key,
			item.env,
			item.value,
			item.defaultVal))
	}
	return strings.Join(s, "\n")
}

// getDelta compares two instances of the same struct and returns the delta.
func getDelta(base, changed interface{}) *Delta {
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
			subDelta := getDelta(fieldBase.Interface(), fieldChanged.Interface())
			delta.add(subDelta.items()...)
		} else {
			if fieldChanged.CanSet() &&
				fieldBase.Interface() != fieldChanged.Interface() {
				kv := DeltaItem{
					key:        typeFieldChanged.Name,
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

// initStruct initialize the struct with the default values of the struct tags
// The input must be an addressable struct object (or its pointer)
func initStruct(c interface{}) interface{} {
	val := reflect.Indirect(reflect.ValueOf(c))

	for i := 0; i < val.NumField(); i++ {
		fieldVal := reflect.Indirect(val.Field(i))
		field := val.Type().Field(i)

		if field.Anonymous || !fieldVal.CanSet() {
			continue
		}
		if fieldVal.Kind() == reflect.Struct {
			initStruct(val.Field(i).Interface())
		} else {
			tagDefault := field.Tag.Get("default")
			setField(c, field, stringToValue(tagDefault, field.Type.Kind()))
		}
	}

	return c
}

// setField assigns `val` to struct c's field specified by the name `field`. It
// first checks if there is a setter method `Set+FieldName` for this field, and
// calls the setter if so. Otherwise if will set the value directly via the
// reflect.Value.Set function.
//
// The setter, if provided, if preferred as there may be some extra work around,
// for example it may need to acquire a mutex, or set some other flags.
// The `val` must have the same dynamic type as the field, otherwise it will panic.
func setField(c interface{}, field reflect.StructField, val reflect.Value) {
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

	setMethodName := fmt.Sprintf("Set%s", field.Name)
	setMethodV := reflect.ValueOf(c).MethodByName(setMethodName)
	// setMethod may be invalid, but we call setMethodV.IsValid() first before
	// all the other methods so it should be safe.
	setMethod, _ := reflect.TypeOf(c).MethodByName(setMethodName)
	setMethodT := setMethod.Type

	// Call the setter if we have found a valid one
	if setMethodV.IsValid() &&
		setMethodT.NumIn() == 2 && // In(0) is the receiver
		setMethodT.In(1).Kind() == fieldKind {
		setMethodV.Call([]reflect.Value{val})
	} else {
		fieldVal.Set(val)
	}
}

// stringToValue converts a string to a value specified by the kind.
func stringToValue(s string, kind reflect.Kind) reflect.Value {
	s = strings.TrimSpace(s)

	var val interface{}
	var err error
	switch kind {
	case reflect.Int:
		if s == "" {
			s = "0"
		}
		val, err = strconv.Atoi(s)
		if err != nil {
			log.Warningf("Ignore invalid int value: %s", s)
		}
	case reflect.Int64:
		if s == "" {
			s = "0"
		}
		val, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			log.Warningf("Ignore invalid int64 value: %s", s)
		}
	case reflect.String:
		val = s
	case reflect.Bool:
		if s == "" {
			s = "false"
		}
		val, err = toBool(s)
		if err != nil {
			log.Warningf("Ignore invalid bool value: %s", errors.Wrap(err, s))
		}
	default:
		panic(fmt.Sprintf("Unsupported kind: %v, val: %s", kind, s))
	}
	return reflect.ValueOf(val)
}

// loadEnvs loads environment variable values and update the Config object.
func (c *Config) loadEnvs() {
	loadEnvsInternal(c)
}

// c must be a pointer to a struct object
func loadEnvsInternal(c interface{}) {
	cv := reflect.Indirect(reflect.ValueOf(c))
	ct := cv.Type()

	if !cv.CanSet() {
		// TODO: log a warning?
		return
	}

	for i := 0; i < ct.NumField(); i++ {
		fieldV := reflect.Indirect(cv.Field(i))
		if !fieldV.CanSet() || ct.Field(i).Anonymous {
			continue
		}

		field := ct.Field(i)
		fieldK := fieldV.Kind()
		if fieldK == reflect.Struct {
			loadEnvsInternal(cv.Field(i).Interface())
		} else {
			tagV := field.Tag.Get("env")
			if tagV == "" {
				continue
			}

			envVal := os.Getenv(tagV)
			if envVal == "" {
				continue
			}

			setField(c, field, stringToValue(envVal, fieldK))
		}
	}
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
