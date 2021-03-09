// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
)

var GlobalConfig = NewConfig()

// GetCollector is a wrapper to the method of the global config
var GetCollector = GlobalConfig.GetCollector

// GetServiceKey is a wrapper to the method of the global config
var GetServiceKey = GlobalConfig.GetServiceKey

// GetTrustedPath is a wrapper to the method of the global config
var GetTrustedPath = GlobalConfig.GetTrustedPath

// GetReporterType is a wrapper to the method of the global config
var GetReporterType = GlobalConfig.GetReporterType

// GetTracingMode is a wrapper to the method of the global config
var GetTracingMode = GlobalConfig.GetTracingMode

// GetSampleRate is a wrapper to the method of the global config
var GetSampleRate = GlobalConfig.GetSampleRate

// SamplingConfigured is a wrapper to the method of the global config
var SamplingConfigured = GlobalConfig.SamplingConfigured

// GetCollectorUDP is a wrapper to the method of the global config
var GetCollectorUDP = GlobalConfig.GetCollectorUDP

// GetPrependDomain is a wrapper to the method of the global config
var GetPrependDomain = GlobalConfig.GetPrependDomain

// GetHostAlias is a wrapper to the method of the global config
var GetHostAlias = GlobalConfig.GetHostAlias

// GetPrecision is a wrapper to the method of the global config
var GetPrecision = GlobalConfig.GetPrecision

// GetDisabled is a wrapper to the method of the global config
var GetDisabled = GlobalConfig.GetDisabled

// ReporterOpts is a wrapper to the method of the global config
var ReporterOpts = GlobalConfig.GetReporter

// GetEc2MetadataTimeout is a wrapper to the method of the global config
var GetEc2MetadataTimeout = GlobalConfig.GetEc2MetadataTimeout

// DebugLevel is a wrapper to the method of the global config
var DebugLevel = GlobalConfig.GetDebugLevel

// GetTriggerTrace is a wrapper to the method of the global config
var GetTriggerTrace = GlobalConfig.GetTriggerTrace

// GetProxy is a wrapper to the method of the global config
var GetProxy = GlobalConfig.GetProxy

// GetProxyCertPath is a wrapper to the method of the global config
var GetProxyCertPath = GlobalConfig.GetProxyCertPath

// GetRuntimeMetrics is a wrapper to the method of the global config
var GetRuntimeMetrics = GlobalConfig.GetRuntimeMetrics

var GetTokenBucketCap = GlobalConfig.GetTokenBucketCap
var GetTokenBucketRate = GlobalConfig.GetTokenBucketRate
var GetReportQueryString = GlobalConfig.GetReportQueryString

// GetTransactionFiltering is a wrapper to the method of the global config
var GetTransactionFiltering = GlobalConfig.GetTransactionFiltering

var GetTransactionName = GlobalConfig.GetTransactionName

// GetSQLSanitize is a wrapper to method GetSQLSanitize of the global variable config.
var GetSQLSanitize = GlobalConfig.GetSQLSanitize

// Load reads the customized configurations
var Load = GlobalConfig.Load

var GetDelta = GlobalConfig.GetDelta

func init() {
	if AutoAgentEnabled() && !GlobalConfig.GetDisabled() {
		log.Warningf("Accepted config items: \n%s", GlobalConfig.GetDelta())
	}
}
