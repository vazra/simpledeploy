.PHONY: build build-go test clean ui-build dev api ui api-non-hmr

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

ui-build:
	cd ui && npm install && npm run build
	rm -rf cmd/simpledeploy/ui_dist
	cp -r ui/dist cmd/simpledeploy/ui_dist

build: ui-build build-go

build-go:
	go build -ldflags="$(LDFLAGS)" -o bin/simpledeploy ./cmd/simpledeploy

test:
	go test ./...

dev:
	go install github.com/air-verse/air@latest
	@trap 'kill %1 %2 2>/dev/null' EXIT; \
	$$(go env GOPATH)/bin/air -c .air.toml & \
	cd ui && npm run dev

api:
	go install github.com/air-verse/air@latest
	$$(go env GOPATH)/bin/air -c .air.toml

ui:
	cd ui && npm run dev

api-non-hmr: build-go
	./bin/simpledeploy serve --config config.dev.yaml

clean:
	rm -rf bin/ cmd/simpledeploy/ui_dist
