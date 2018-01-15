coverpkg="github.com/appoptics/appoptics-apm-go/v1/tv/internal/reporter,github.com/appoptics/appoptics-apm-go/v1/tv,github.com/appoptics/appoptics-apm-go/v1/tv/ottv"
cov_args=-covermode=atomic -coverpkg=$(coverpkg)
cov_out=-coverprofile=cov.out 
covtv_out=-coverprofile=covtv.out 
cov_files=cov.out covtv.out internal/reporter/cov.out internal/reporter/covtv.out ottv/cov.out ottv/covtv.out
cov_merge=covmerge.out
test_args=-race

test: export APPOPTICS_DEBUG_LEVEL=1
#test: export APPOPTICS_DEBUG_LEVEL=0
test:
	go test $(test_args) $(cov_args) -v $(cov_out)
	go test $(test_args) $(cov_args) -v $(covtv_out) -tags disable_tracing
	cd internal/reporter && go test $(test_args) $(cov_args) $(cov_out) -v
	cd internal/reporter && go test $(test_args) $(cov_args) $(covtv_out) -v -tags disable_tracing
	cd ottv && go test $(test_args) $(cov_args) $(cov_out) -v
	cd ottv && go test $(test_args) $(cov_args) $(covtv_out) -v -tags disable_tracing

$(cov_merge): test $(cov_files)
	gocovmerge $(cov_files) > $(cov_merge)

coverhtml: $(cov_merge)
	go tool cover -html=$(cov_merge)
