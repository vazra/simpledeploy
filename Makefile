.PHONY: build build-go test clean ui-build

ui-build:
	cd ui && npm install && npm run build
	rm -rf cmd/simpledeploy/ui_dist
	cp -r ui/dist cmd/simpledeploy/ui_dist

build: ui-build build-go

build-go:
	go build -ldflags="-s -w" -o bin/simpledeploy ./cmd/simpledeploy

test:
	go test ./...

clean:
	rm -rf bin/ cmd/simpledeploy/ui_dist
