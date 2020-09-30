certgen:
	@cd /Users/yang/go/src/github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter && ./certgen.sh

runtest:
	@cd v1 && go test -race -timeout 3m -short ./... && echo "All tests passed."

removecert:
	@cd /Users/yang/go/src/github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter && rm for_test.crt for_test.key

test: certgen runtest removecert

examples:
	@cd examples && go test -race -timeout 1m -short ./... && echo "All examples passed."

vet: 
	@go vet -composites=false ./... && echo "Go vet analysis passed."

sure: test examples vet

.PHONY: certgen test removecert examples vet
