.PHONY: build test clean

build:
	go build -ldflags="-s -w" -o bin/simpledeploy ./cmd/simpledeploy

test:
	go test ./...

clean:
	rm -rf bin/
