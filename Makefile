.PHONY: build test fmt docs docs-check coverage clean staticcheck

build: docs
	go build -o jbs ./cmd/jbs

test: docs-check
	go test ./...

fmt:
	go fmt ./...

docs:
	go run ./cmd/gendiagdocs

docs-check:
	go run ./cmd/gendiagdocs -check

staticcheck:
	staticcheck ./...

coverage:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | awk '/^total:/ {print "Overall coverage: " $$3}'

all: fmt build test coverage

clean:
	rm jbs coverage.out coverage.html
