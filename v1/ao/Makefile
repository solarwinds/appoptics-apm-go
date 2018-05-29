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

are-you-sure:
	@read -p "I suppose you know what you are doing. Are you sure? [Y/n]" -n 1 -r; \
	if [[ $$REPLY != "Y" ]]; then \
		echo -e -n "\nSee you. >> "; \
		exit 1; \
	fi
	@echo -e "\nGood, proceeding..."; \
		$(eval confirmed := "Yes")

# Change the value of initVersion and tag it with the version number.
# Usage: make VERSION=1.2.3 tag-and-release
tag-and-release: are-you-sure
	# VERSION must be set
	@if [ -z "$$VERSION" ]; then \
		echo -e "The VERSION is unset or an empty string"; \
		exit 1; \
	fi
	# Let you know what you are doing.
	@echo -e "Changing the version number and tag it with ${VERSION}."
	# Stash your current work, in case of any
	@git stash
	# We tag and release on the master branch
	@git checkout master
	# Make sure it's up-to-date
	@git fetch origin
	@git reset --hard origin/master
	# Replace the version with the value provided by you
	@sed -i.bak -e 's/const initVersion = ".*"/const initVersion = "${VERSION}"/' internal/reporter/version.go \
		&& rm internal/reporter/version.go.bak
	# And commit it
	@git add internal/reporter/version.go
	@git commit -m "Changed version to ${VERSION}"
	# Now we create tags as a release
	@git tag -a ${VERSION} -m "Version: ${VERSION}"
	# Push it to the remote master branch
	@git push origin master --follow-tags
	# Resume your workspace
	@git checkout ${prev_branch}
	@git stash pop
