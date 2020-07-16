module github.com/appoptics/appoptics-apm-go/v1/contrib/lambda

go 1.14

require (
	github.com/appoptics/appoptics-apm-go v1.13.3
	github.com/aws/aws-lambda-go v1.17.0
	github.com/pkg/errors v0.9.1
)

replace github.com/appoptics/appoptics-apm-go => ../../../
