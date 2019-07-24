test:
	go test -race -timeout 3m -short ./... && echo "All tests passed."
