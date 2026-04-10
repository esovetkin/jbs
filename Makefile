.PHONY: build test clean

build:
	go build -o jbs ./cmd/jbs

test:
	go test ./...

coverage:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm jbs
