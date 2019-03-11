// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"sync/atomic"
)

// ReporterOptions defines the options of a reporter. The fields of it
// must be accessed through atomic operators
type ReporterOptions struct {
	// Events flush interval in seconds
	EvtFlushInterval int64 `yaml:",omitempty" env:"APPOPTICS_EVENTS_FLUSH_INTERVAL" default:"2"`

	// Event sending batch size in KB
	EvtFlushBatchSize int64 `yaml:",omitempty" env:"APPOPTICS_EVENTS_BATCHSIZE" default:"2000"`

	// Metrics flush interval in seconds
	MetricFlushInterval int64 `yaml:",omitempty" default:"30"`

	// GetSettings interval in seconds
	GetSettingsInterval int64 `yaml:",omitempty" default:"30"`

	// Settings timeout interval in seconds
	SettingsTimeoutInterval int64 `yaml:",omitempty" default:"10"`

	// Ping interval in seconds
	PingInterval int64 `yaml:",omitempty" default:"20"`

	// Retry backoff initial delay
	RetryDelayInitial int64 `yaml:",omitempty" default:"500"`

	// Maximum retry delay
	RetryDelayMax int `yaml:",omitempty" default:"60"`

	// Maximum redirect times
	RedirectMax int `yaml:",omitempty" default:"20"`

	// The threshold of retries before debug printing
	RetryLogThreshold int `yaml:",omitempty" default:"10"`

	// The maximum retries
	MaxRetries int `yaml:",omitempty" default:"20"`
}

// SetEventFlushInterval sets the event flush interval to i
func (r *ReporterOptions) SetEventFlushInterval(i int64) {
	atomic.StoreInt64(&r.EvtFlushInterval, i)
}

// SetEventBatchSize sets the event flush interval to i
func (r *ReporterOptions) SetEventBatchSize(i int64) {
	atomic.StoreInt64(&r.EvtFlushBatchSize, i)
}

// GetEventFlushInterval returns the current event flush interval
func (r *ReporterOptions) GetEventFlushInterval() int64 {

	return atomic.LoadInt64(&r.EvtFlushInterval)
}

// GetEventBatchSize returns the current event flush interval
func (r *ReporterOptions) GetEventBatchSize() int64 {

	return atomic.LoadInt64(&r.EvtFlushBatchSize)
}

// LoadEnvs load environment variables and refresh reporter options.
func (r *ReporterOptions) loadEnvs() {
	i := Env(envAppOpticsEventsFlushInterval).ToInt64(r.EvtFlushInterval)
	r.SetEventFlushInterval(i)

	b := Env(envAppOpticsEventsBatchSize).ToInt64(r.EvtFlushBatchSize)
	r.SetEventBatchSize(b)
}

func (r *ReporterOptions) validate() error {
	// TODO
	return nil
}
