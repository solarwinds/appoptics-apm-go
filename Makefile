test:
	@go test -race -timeout 3m -short ./... && echo "All tests passed."

vet: 
	@go vet -composites=false ./... && echo "Go vet analysis passed."
