SHELL=bash

prev_branch :=$(shell git rev-parse --abbrev-ref HEAD)

coverpkg="github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter,github.com/appoptics/appoptics-apm-go/v1/ao,github.com/appoptics/appoptics-apm-go/v1/ao/opentracing"
cov_args=-covermode=atomic -coverpkg=$(coverpkg)
cov_out=-coverprofile=cov.out
covao_out=-coverprofile=covao.out
cov_files=cov.out covao.out internal/reporter/cov.out internal/reporter/covao.out opentracing/cov.out opentracing/covao.out
cov_merge=covmerge.out
test_args=-race

test: export APPOPTICS_DEBUG_LEVEL=1
#test: export APPOPTICS_DEBUG_LEVEL=0
test:
	go test $(test_args) $(cov_args) -v $(cov_out)
	go test $(test_args) $(cov_args) -v $(covao_out) -tags disable_tracing
	cd internal/reporter && go test $(test_args) $(cov_args) $(cov_out) -v
	cd internal/reporter && go test $(test_args) $(cov_args) $(covao_out) -v -tags disable_tracing
	cd opentracing && go test $(test_args) $(cov_args) $(cov_out) -v
	cd opentracing && go test $(test_args) $(cov_args) $(covao_out) -v -tags disable_tracing

$(cov_merge): test $(cov_files)
	gocovmerge $(cov_files) > $(cov_merge)

coverhtml: $(cov_merge)
	go tool cover -html=$(cov_merge)