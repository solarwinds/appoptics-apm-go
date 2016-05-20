#!/bin/bash
set -e

COVERPKG="github.com/appneta/go-appneta/v1/tv/internal/traceview,github.com/appneta/go-appneta/v1/tv"
go test -v -covermode=count -coverprofile=cov.out -coverpkg $COVERPKG
go test -v -tags traceview -covermode=count -coverprofile=covtv.out -coverpkg $COVERPKG

pushd internal/traceview/
go test -v -covermode=count -coverprofile=cov.out
go test -v -tags traceview -covermode=count -coverprofile=covtv.out
popd

gocovmerge cov.out covtv.out internal/traceview/cov.out internal/traceview/covtv.out > covmerge.out

#go tool cover -html=covmerge.out
