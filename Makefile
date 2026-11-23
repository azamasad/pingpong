.PHONY: build test lint tidy clean

build:
	go build -o bin/pingpong ./cmd/pingpong

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o bin/pingpong-darwin-arm64 ./cmd/pingpong

test:
	go test ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

tidy:
	go mod tidy

clean:
	rm -rf bin/