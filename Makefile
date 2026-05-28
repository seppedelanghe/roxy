.PHONY: build test vet fmt run

build:
	go build -o bin/roxy ./cmd/roxy

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run: build
	./bin/roxy
