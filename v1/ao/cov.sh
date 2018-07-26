#!/bin/bash
set -e

COVERPKG="github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter,github.com/appoptics/appoptics-apm-go/v1/ao/internal/log,github.com/appoptics/appoptics-apm-go/v1/ao,github.com/appoptics/appoptics-apm-go/v1/ao/opentracing"
export APPOPTICS_DEBUG_LEVEL=1
go test -v -race -covermode=atomic -coverprofile=cov.out -coverpkg $COVERPKG
go test -v -race -tags disable_tracing -covermode=atomic -coverprofile=covao.out -coverpkg $COVERPKG

pushd internal/reporter/
go test -v -race -covermode=atomic -coverprofile=cov.out
go test -v -race -tags disable_tracing -covermode=atomic -coverprofile=covao.out
popd

pushd internal/log/
go test -v -race -covermode=atomic -coverprofile=cov.out
go test -v -race -tags disable_tracing -covermode=atomic -coverprofile=covao.out
popd

pushd opentracing
go test -v -race -covermode=atomic -coverprofile=cov.out
go test -v -race -tags disable_tracing -covermode=atomic -coverprofile=covao.out
popd

gocovmerge cov.out covao.out internal/reporter/cov.out internal/reporter/covao.out internal/log/cov.out internal/log/covao.out opentracing/cov.out opentracing/covao.out > covmerge.out

#go tool cover -html=covmerge.out
