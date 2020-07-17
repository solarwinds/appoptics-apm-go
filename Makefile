test:
	@cd v1 && go test -race -timeout 3m -short ./... && echo "All tests passed."

examples:
	@cd examples && go test -race -timeout 1m -short ./... && echo "All examples passed."

contrib:
	@cd v1/contrib/ && go test -race -timeout 1m -short ./... && echo "Contrib tests passed."

vet: 
	@go vet -composites=false ./... && echo "Go vet analysis passed."

clean:
	@go clean -testcache ./...

sure: test examples contrib vet

.PHONY: test examples vet contrib clean
