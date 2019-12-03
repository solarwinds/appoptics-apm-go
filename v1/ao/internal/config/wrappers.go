// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import "github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"

var conf = NewConfig()

// GetCollector is a wrapper to the method of the global config
var GetCollector = conf.GetCollector

// GetServiceKey is a wrapper to the method of the global config
var GetServiceKey = conf.GetServiceKey

// GetTrustedPath is a wrapper to the method of the global config
var GetTrustedPath = conf.GetTrustedPath

// GetReporterType is a wrapper to the method of the global config
var GetReporterType = conf.GetReporterType

// GetTracingMode is a wrapper to the method of the global config
var GetTracingMode = conf.GetTracingMode

// GetSampleRate is a wrapper to the method of the global config
var GetSampleRate = conf.GetSampleRate

// SamplingConfigured is a wrapper to the method of the global config
var SamplingConfigured = conf.SamplingConfigured

// GetCollectorUDP is a wrapper to the method of the global config
var GetCollectorUDP = conf.GetCollectorUDP

// GetPrependDomain is a wrapper to the method of the global config
var GetPrependDomain = conf.GetPrependDomain

// GetHostAlias is a wrapper to the method of the global config
var GetHostAlias = conf.GetHostAlias

// GetSkipVerify is a wrapper to the method of the global config
var GetSkipVerify = conf.GetSkipVerify

// GetPrecision is a wrapper to the method of the global config
var GetPrecision = conf.GetPrecision

// GetDisabled is a wrapper to the method of the global config
var GetDisabled = conf.GetDisabled

// ReporterOpts is a wrapper to the method of the global config
var ReporterOpts = conf.GetReporter

// GetEc2MetadataTimeout is a wrapper to the method of the global config
var GetEc2MetadataTimeout = conf.GetEc2MetadataTimeout

// DebugLevel is a wrapper to the method of the global config
var DebugLevel = conf.GetDebugLevel

// GetTriggerTrace is a wrapper to the method of the global config
var GetTriggerTrace = conf.GetTriggerTrace

// GetTransactionFiltering is a wrapper to the method of the global config
var GetTransactionFiltering = conf.GetTransactionFiltering

// GetSQLSanitize is a wrapper to method GetSQLSanitize of the global variable config.
var GetSQLSanitize = conf.GetSQLSanitize

// Load reads the customized configurations
var Load = conf.Load

var GetDelta = conf.GetDelta

func init() {
	if !conf.GetDisabled() {
		log.Warningf("Accepted config items: \n%s", conf.GetDelta())
	}
}
