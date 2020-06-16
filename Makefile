test:
	@cd v1 && go test -race -timeout 3m -short ./... && echo "All tests passed."

examples:
	@cd examples && go test -race -timeout 1m -short ./... && echo "All examples passed."

vet: 
	@go vet -composites=false ./... && echo "Go vet analysis passed."

sure: test examples vet

.PHONY: test examples vet
