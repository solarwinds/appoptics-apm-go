module github.com/appoptics/appoptics-apm-go/examples

go 1.14

require (
	github.com/appoptics/appoptics-apm-go v1.14.0
	github.com/gin-gonic/gin v1.7.0
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/opentracing/opentracing-go v1.1.0
	github.com/stretchr/testify v1.7.0
)

replace github.com/appoptics/appoptics-apm-go => ../
