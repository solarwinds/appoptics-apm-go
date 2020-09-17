module github.com/appoptics/appoptics-apm-go/v1/contrib

go 1.14

require (
	github.com/appoptics/appoptics-apm-go v1.13.3
	github.com/aws/aws-lambda-go v1.19.1
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	google.golang.org/grpc v1.30.0
)

replace github.com/appoptics/appoptics-apm-go => ../../
