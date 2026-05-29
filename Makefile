install:
	go install github.com/vektra/mockery/v3@v3.7.0

test:
	go vet ./...
	go test -v -cover ./...

lint:
	golangci-lint run ./...

ci: test lint

clean:
	go clean ./...

mocks:
	@mockery

.PHONY: test lint ci mocks