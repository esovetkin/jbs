.PHONY: build test clean

build:
	go build -o jbs ./cmd/jbs

test:
	go test ./...

clean:
	rm jbs
