module github.com/appoptics/appoptics-apm-go/examples

go 1.14

require (
	github.com/appoptics/appoptics-apm-go v1.14.0
	github.com/gin-gonic/gin v1.6.3
	github.com/opentracing/opentracing-go v1.1.0
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin v0.19.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.19.0
	go.opentelemetry.io/otel v0.19.0
	go.opentelemetry.io/otel/trace v0.19.0
)

replace github.com/appoptics/appoptics-apm-go => ../
