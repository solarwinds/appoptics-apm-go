test:
	go test -v -race -timeout 3m ./... && echo "All tests passed."
