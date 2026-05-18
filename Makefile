.PHONY: build test fmt coverage clean staticcheck all

build:
	go build -buildvcs=true -o jbs .

test:
	go test ./...

fmt:
	go fmt ./...

coverage:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | awk '/^total:/ {print "Overall coverage: " $$3}'

clean:
	rm jbs coverage.out coverage.html

staticcheck:
	staticcheck ./...

all: fmt build test staticcheck coverage
