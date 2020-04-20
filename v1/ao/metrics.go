// Copyright (C) 2019 Librato, Inc. All rights reserved.

package ao

import (
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
)

// MetricOptions is a struct for the optional parameters of a measurement.
type MetricOptions = metrics.MetricOptions

const (
	// MaxTagsCount is the maximum number of tags allowed.
	MaxTagsCount = metrics.MaxTagsCount
)

// The measurements submission errors
var (
	// ErrExceedsTagsCountLimit indicates the count of tags exceeds the limit
	ErrExceedsTagsCountLimit = metrics.ErrExceedsTagsCountLimit
	// ErrExceedsMetricsCountLimit indicates there are too many distinct measurements in a flush cycle.
	ErrExceedsMetricsCountLimit = metrics.ErrExceedsMetricsCountLimit
	// ErrMetricsWithNonPositiveCount indicates the count is negative or zero
	ErrMetricsWithNonPositiveCount = metrics.ErrMetricsWithNonPositiveCount
)

// SummaryMetric submits a summary type measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func SummaryMetric(name string, value float64, opts MetricOptions) error {
	return reporter.SummaryMetric(name, value, opts)
}

// IncrementMetric submits a incremental measurement to the reporter. The measurements
// will be collected in the background and reported periodically.
func IncrementMetric(name string, opts MetricOptions) error {
	return reporter.IncrementMetric(name, opts)
}
