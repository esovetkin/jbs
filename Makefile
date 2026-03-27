.PHONY: build test clean

GOCACHE ?= /tmp/go-build
GOMODCACHE ?= /tmp/go-mod
GO ?= go

build:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o jbs ./cmd/jbs

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./...

clean:
	rm jbs
