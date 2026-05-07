.PHONY: build test fmt coverage clean staticcheck all

build:
	go build -o jbs .

test:
	go test ./...

fmt:
	go fmt ./...

staticcheck:
	staticcheck ./...

coverage:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | awk '/^total:/ {print "Overall coverage: " $$3}'

all: fmt build test coverage

clean:
	rm jbs coverage.out coverage.html
