.PHONY: check vet test build lint

check: vet test build lint

vet:
	go vet ./...

test:
	go test ./...

build:
	go build -o bin/furrow ./cmd/furrow

lint: build
	./bin/furrow lint
