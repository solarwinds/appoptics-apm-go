package agent

import (
	"fmt"
	"os"

	"strings"
)

type configBuilder struct {
	name         ConfName
	defaultValue string
	builders     []initFunc
	// TODO: validator func
}

type conf struct {
	initialized bool
	items       map[ConfName]string
}

type initFunc func(n ConfName) string

// Environment variable reader. Empty string is considered as invalid so just use os.Getenv()
// to ignore empty environment variables
var envVar initFunc = func(n ConfName) string {
	return strings.ToLower(os.Getenv(string(n)))
}

// Default values
const (
	defaultSSLCollector       = "collector.appoptics.com:443"
	defaultServiceKey         = ""
	defaultDebugLevel         = "WARN"
	defaultTrustedPath        = ""
	defaultCollectorUDP       = "127.0.0.1:7831"
	defaultReporter           = "ssl"
	defaultTracingMode        = "always"
	defaultPrependDomain      = "false"
	defaultHostnameAlias      = ""
	defaultInsecureSkipVerify = "false"
	defaultHistogramPrecision = ""
)

var cb = []configBuilder{
	{AppOpticsCollector, defaultSSLCollector, []initFunc{envVar}},
	{AppOpticsServiceKey, defaultServiceKey, []initFunc{envVar}},
	{AppOpticsDebugLevel, defaultDebugLevel, []initFunc{envVar}},
	{AppOpticsTrustedPath, defaultTrustedPath, []initFunc{envVar}},
	{AppOpticsCollectorUDP, defaultCollectorUDP, []initFunc{envVar}},
	{AppOpticsReporter, defaultReporter, []initFunc{envVar}},
	{AppOpticsTracingMode, defaultTracingMode, []initFunc{envVar}},
	{AppOpticsPrependDomain, defaultPrependDomain, []initFunc{envVar}},
	{AppOpticsHostnameAlias, defaultHostnameAlias, []initFunc{envVar}},
	{AppOpticsInsecureSkipVerify, defaultInsecureSkipVerify, []initFunc{envVar}},
	{AppOpticsHistogramPrecision, defaultHistogramPrecision, []initFunc{envVar}},
}

// The package variable to store all configurations, which is read only after initialized.
var agentConf = conf{
	initialized: false,
	items:       make(map[ConfName]string),
}

func initConf(cf *conf) {
	Log(INFO, "initializing the AppOptics agent")
	for _, item := range cb {
		k := item.name
		v := ""
		l := len(item.builders) - 1
		for i := l; i >= 0; i-- {
			v = item.builders[i](k)
			if v != "" {
				Log(WARNING, fmt.Sprintf("non-default configuration used %v=%v", k, v))
				break
			}
		}
		if v == "" {
			v = item.defaultValue
		}
		cf.items[k] = v
	}
	cf.initialized = true
}
