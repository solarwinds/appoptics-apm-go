// Copyright (C) 2019 Librato, Inc. All rights reserved.

package ao

import (
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
)

type MetricOptions = metrics.MetricOptions

func SummaryMetric(name string, value float32, opts MetricOptions) {
	reporter.SummaryMetric(name, value, opts)
}

func IncrementMetric(name string, opts MetricOptions) {
	reporter.IncrementMetric(name, opts)
}
