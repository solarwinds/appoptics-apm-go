// Copyright (C) 2019 Librato, Inc. All rights reserved.

package ao

import (
	"errors"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
)

type MetricOptions = metrics.MetricOptions

func SummaryMetric(name string, value float64, opts MetricOptions) error {
	if len(opts.Tags) > 50 {
		return errors.New("exceeds tags count limit")
	}
	return reporter.SummaryMetric(name, value, opts)
}

func IncrementMetric(name string, opts MetricOptions) error {
	if len(opts.Tags) > 50 {
		return errors.New("exceeds tags count limit")
	}
	return reporter.IncrementMetric(name, opts)
}
