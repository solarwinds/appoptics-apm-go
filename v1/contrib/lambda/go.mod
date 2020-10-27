module github.com/appoptics/appoptics-apm-go/v1/contrib/lambda

go 1.14

require (
	github.com/appoptics/appoptics-apm-go v1.13.4
	github.com/aws/aws-lambda-go v1.20.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
)

replace github.com/appoptics/appoptics-apm-go => ../../../