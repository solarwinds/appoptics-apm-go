certgen:
	@cd v1/ao/internal/reporter && ./certgen.sh

runtest:
	@cd v1 && go test -race -timeout 3m -count=1 -short ./... && echo "All tests passed."

runtestfast:
	@cd v1 && go test -race -timeout 3m -short ./... && echo "All tests passed."

removecert:
	@cd v1/ao/internal/reporter && rm for_test.crt for_test.key

test: certgen runtest removecert
testfast: certgen runtestfast removecert

examples:
	@cd examples && go test -race -timeout 1m -short ./... && echo "All examples passed."

CONTRIB = v1/contrib
contrib: $(CONTRIB)/*
	@for dir in $^ ; do \
		cd $$dir && go test -race -timeout 1m -short ./... && cd ~-; \
	done && echo "Contrib tests passed"

vet:
	@go vet -composites=false ./... && echo "Go vet analysis passed."

clean:
	@go clean -testcache ./...

sure: clean test examples contrib vet

.PHONY: certgen test removecert examples vet contrib clean
