#!/bin/bash
set -e

COVERPKG="github.com/librato/go-traceview/v1/tv/internal/traceview,github.com/librato/go-traceview/v1/tv,github.com/librato/go-traceview/v1/tv/ottv"
export TRACEVIEW_DEBUG=1
go test -v -covermode=count -coverprofile=cov.out -coverpkg $COVERPKG
go test -v -tags traceview -covermode=count -coverprofile=covtv.out -coverpkg $COVERPKG

pushd internal/traceview/
go test -v -covermode=count -coverprofile=cov.out
go test -v -tags traceview -covermode=count -coverprofile=covtv.out
popd

pushd ottv
go test -v -covermode=count -coverprofile=cov.out
go test -v -tags traceview -covermode=count -coverprofile=covtv.out
popd

gocovmerge cov.out covtv.out internal/traceview/cov.out internal/traceview/covtv.out ottv/cov.out ottv/covtv.out > covmerge.out

#go tool cover -html=covmerge.out
