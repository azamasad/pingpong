.PHONY: build test lint tidy clean

build:
	go build -o bin/pingpong ./cmd/pingpong

test:
	go test ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

tidy:
	go mod tidy

clean:
	rm -rf bin/