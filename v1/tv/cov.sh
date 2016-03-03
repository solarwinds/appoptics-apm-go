#!/bin/bash
go test -v -covermode=count -coverprofile=cov.out \
   -coverpkg github.com/appneta/go-traceview/v1/tv/internal/traceview,github.com/appneta/go-traceview/v1/tv

go test -v -tags traceview -covermode=count -coverprofile=covtv.out \
   -coverpkg github.com/appneta/go-traceview/v1/tv/internal/traceview,github.com/appneta/go-traceview/v1/tv

pushd internal/traceview/
go test -v -covermode=count -coverprofile=cov.out
go test -v -tags traceview -covermode=count -coverprofile=covtv.out
popd

gocovmerge cov.out covtv.out internal/traceview/cov.out internal/traceview/covtv.out > covmerge.out

go tool cover -html=covmerge.out
