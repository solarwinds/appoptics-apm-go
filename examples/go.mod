module github.com/appoptics/appoptics-apm-go/examples

go 1.14

require (
	github.com/appoptics/appoptics-apm-go latest
	github.com/gin-gonic/gin v1.6.3
	github.com/opentracing/opentracing-go v1.1.0
	github.com/stretchr/testify v1.6.1
)

replace github.com/appoptics/appoptics-apm-go => ../
