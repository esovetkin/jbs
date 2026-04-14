.PHONY: build test fmt docs docs-check coverage clean

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

coverage:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm jbs coverage.out coverage.html
