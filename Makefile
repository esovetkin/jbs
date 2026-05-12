.PHONY: build test fmt coverage clean staticcheck all

MODULE := gitlab.jsc.fz-juelich.de/sdlaml/jbs
VERSION ?= v0.1.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X '$(MODULE)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/version.BuildDate=$(BUILD_DATE)'

build:
	go build -ldflags "$(LDFLAGS)" -o jbs .

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
