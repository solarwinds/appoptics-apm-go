// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"sync/atomic"
)

// default values of the reporter parameters
const (
	// the default interval in seconds to flush events to the collector
	eventFlushIntervalDefault = 2

	// the default message batch size (in KB) for each RPC call
	eventFlushBatchSizeDefault = 2000

	// the default interval in seconds to flush metrics
	metricIntervalDefault = 30

	// the default interval in seconds to get settings from collector
	getSettingsIntervalDefault = 30

	// the default settings TTL check interval in seconds
	settingsTimeoutCheckIntervalDefault = 10

	// the default ping interval in seconds
	pingIntervalDefault = 20

	// the default initial retry delay in milliseconds
	retryDelayInitial = 500

	// the default retry delay multiplier for back-off
	retryDelayMultiplier = 1.5

	// the maximum retry delay time in seconds
	retryDelayMax = 60

	// the maximum redirects
	redirectMax = 20

	// the threshold of retries before logging a warning
	retryLogThreshold = 10

	// the maximum delays
	maxRetries = 20
)

// ReporterOptions defines the options of a reporter. The fields of it
// must be accessed through atomic operators
type ReporterOptions struct {
	// Events flush interval in seconds
	EvtFlushInterval int64

	// Event sending batch size in KB
	EvtFlushBatchSize int64

	// Metrics flush interval in seconds
	MetricFlushInterval int64

	// GetSettings interval in seconds
	GetSettingsInterval int64

	// Settings timeout interval in seconds
	SettingsTimeoutInterval int64

	// Ping interval in seconds
	PingInterval int64

	// Retry backoff initial delay
	RetryDelayInitial int64

	// Maximum retry delay
	RetryDelayMax int

	// Maximum redirect times
	RedirectMax int

	// The threshold of retries before debug printing
	RetryLogThreshold int

	// The maximum retries
	MaxRetries int
}

// defaultReporterOptions creates an ReporterOptions object with the
// default values.
func defaultReporterOptions() *ReporterOptions {
	return &ReporterOptions{
		EvtFlushInterval:        eventFlushIntervalDefault,
		EvtFlushBatchSize:       eventFlushBatchSizeDefault,
		MetricFlushInterval:     metricIntervalDefault,
		GetSettingsInterval:     getSettingsIntervalDefault,
		SettingsTimeoutInterval: settingsTimeoutCheckIntervalDefault,
		PingInterval:            pingIntervalDefault,
		RetryDelayInitial:       retryDelayInitial,
		RetryDelayMax:           retryDelayMax,
		RedirectMax:             redirectMax,
		RetryLogThreshold:       retryLogThreshold,
		MaxRetries:              maxRetries,
	}
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
func (r *ReporterOptions) LoadEnvs() {
	i := envs["EventsFlushInterval"].LoadInt64(r.EvtFlushInterval)
	r.SetEventFlushInterval(i)

	b := envs["EventsBatchSize"].LoadInt64(r.EvtFlushBatchSize)
	r.SetEventBatchSize(b)
}
