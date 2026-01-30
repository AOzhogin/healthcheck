install:
	go install github.com/vektra/mockery/v2@v2.42.1

test:
	go vet ./...
	go test -v -cover ./...

lint:
	golangci-lint run ./...

ci: test lint

mocks:
	@mockery

.PHONY: test lint ci mocks