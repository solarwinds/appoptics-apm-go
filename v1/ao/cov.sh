#!/bin/bash
set -e

COVERPKG="github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter,github.com/appoptics/appoptics-apm-go/v1/ao,github.com/appoptics/appoptics-apm-go/v1/ao/ottv"
export APPOPTICS_DEBUG_LEVEL=0
go test -v -covermode=count -coverprofile=cov.out -coverpkg $COVERPKG
go test -v -tags disable_tracing -covermode=count -coverprofile=covtv.out -coverpkg $COVERPKG

pushd internal/reporter/
go test -v -covermode=count -coverprofile=cov.out
go test -v -tags disable_tracing -covermode=count -coverprofile=covtv.out
popd

pushd ottv
go test -v -covermode=count -coverprofile=cov.out
go test -v -tags disable_tracing -covermode=count -coverprofile=covtv.out
popd

gocovmerge cov.out covtv.out internal/reporter/cov.out internal/reporter/covtv.out ottv/cov.out ottv/covtv.out > covmerge.out

#go tool cover -html=covmerge.out
