install:
	go install github.com/vektra/mockery/v2@v2.42.1

test:
	go vet ./...
	go test -v -cover ./...

mocks:
	@mockery

.PHONY: test