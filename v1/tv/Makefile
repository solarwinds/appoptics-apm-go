coverpkg="github.com/librato/go-traceview/v1/tv/internal/traceview,github.com/librato/go-traceview/v1/tv,github.com/librato/go-traceview/v1/tv/ottv"
cov_args=-covermode=atomic -coverpkg=$(coverpkg)
cov_out=-coverprofile=cov.out 
covtv_out=-coverprofile=covtv.out 
cov_files=cov.out covtv.out internal/traceview/cov.out internal/traceview/covtv.out ottv/cov.out ottv/covtv.out
cov_merge=covmerge.out
test_args=-race

test: export TRACEVIEW_DEBUG=1
test:
	go test $(test_args) $(cov_args) -v $(cov_out)
	go test $(test_args) $(cov_args) -v $(covtv_out) -tags traceview
	cd internal/traceview && go test $(test_args) $(cov_args) $(cov_out) -v
	cd internal/traceview && go test $(test_args) $(cov_args) $(covtv_out) -v -tags tracevie
	cd ottv && go test $(test_args) $(cov_args) $(cov_out) -v
	cd ottv && go test $(test_args) $(cov_args) $(covtv_out) -v -tags traceview

$(cov_merge): test $(cov_files)
	gocovmerge $(cov_files) > $(cov_merge)

coverhtml: $(cov_merge)
	go tool cover -html=$(cov_merge)
