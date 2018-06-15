// This package is used to initialize and fetch the static conf options
// which will not be changed once initialized.
// So mutexes/locks are not required to access the values
package agent

type ConfName string

// All the configuration items currently supported
const (
	AppOpticsCollector          = ConfName("APPOPTICS_COLLECTOR")
	AppOpticsServiceKey         = ConfName("APPOPTICS_SERVICE_KEY")
	AppOpticsDebugLevel         = ConfName("APPOPTICS_DEBUG_LEVEL")
	AppOpticsTrustedPath        = ConfName("APPOPTICS_TRUSTEDPATH")
	AppOpticsCollectorUDP       = ConfName("APPOPTICS_COLLECTOR_UDP")
	AppOpticsReporter           = ConfName("APPOPTICS_REPORTER")
	AppOpticsTracingMode        = ConfName("APPOPTICS_TRACING_MODE")
	AppOpticsPrependDomain      = ConfName("APPOPTICS_PREPEND_DOMAIN")
	AppOpticsHostnameAlias      = ConfName("APPOPTICS_HOSTNAME_ALIAS")
	AppOpticsInsecureSkipVerify = ConfName("APPOPTICS_INSECURE_SKIP_VERIFY")
	AppOpticsHistogramPrecision = ConfName("APPOPTICS_HISTOGRAM_PRECISION")
)

// GetConfig returns the value of a configuration item. Empty string will be returned
// if the item is unset or non-exist and the returned value is ensured lowercase
func GetConfig(n ConfName) string {
	if !agentConf.initialized {
		return ""
	}
	v, ok := agentConf.items[n]
	if !ok {
		return ""
	}
	return v
}
