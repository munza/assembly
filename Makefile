.PHONY: build test
build:
	go build -o .assembly/bin/foreman ./cmd/foreman

test:
	go test ./...
