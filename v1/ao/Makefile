SHELL=bash

prev_branch :=$(shell git rev-parse --abbrev-ref HEAD)

coverpkg="github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter,github.com/appoptics/appoptics-apm-go/v1/ao/internal/log,github.com/appoptics/appoptics-apm-go/v1/ao/internal/bson,github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics,github.com/appoptics/appoptics-apm-go/v1/ao,github.com/appoptics/appoptics-apm-go/v1/ao/internal/config,github.com/appoptics/appoptics-apm-go/v1/ao/internal/host,github.com/appoptics/appoptics-apm-go/v1/contrib/multitracer"
cov_args=-covermode=atomic -coverpkg=$(coverpkg)
cov_out=-coverprofile=cov.out
cov_files=cov.out internal/reporter/cov.out opentracing/cov.out
cov_merge=covmerge.out
test_args=-race

test: export APPOPTICS_DEBUG_LEVEL=1
#test: export APPOPTICS_DEBUG_LEVEL=0
test:
	go test $(test_args) $(cov_args) -v $(cov_out)
	cd internal/reporter && go test $(test_args) $(cov_args) $(cov_out) -v
	cd opentracing && go test $(test_args) $(cov_args) $(cov_out) -v

$(cov_merge): test $(cov_files)
	gocovmerge $(cov_files) > $(cov_merge)

coverhtml: $(cov_merge)
	go tool cover -html=$(cov_merge)
