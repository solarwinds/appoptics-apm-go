// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

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

// HasLocalSamplingConfig is a wrapper to the method of the global config
var HasLocalSamplingConfig = conf.HasLocalSamplingConfig

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

// Refresh reloads the customized configurations
var Refresh = conf.RefreshConfig
