.PHONY: build build-go test clean ui-build dev api ui api-non-hmr e2e e2e-lite e2e-headed e2e-report e2e-templates

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

# E2E Testing (global-setup.js runs make build internally)
e2e:
	cd e2e && npm ci && npx playwright install chromium && npx playwright test

# Lite E2E: skips slow specs (DB strategies, S3 backup, webhook formats,
# private registry). ~6-8 min vs ~20+ min for full suite. Good for local
# dev loop; CI should still run full e2e.
e2e-lite:
	cd e2e && npm ci && npx playwright install chromium && E2E_LITE=1 npx playwright test

e2e-headed:
	cd e2e && npm ci && npx playwright install chromium && npx playwright test --headed

e2e-report:
	cd e2e && npx playwright show-report

# Deploy every app template end-to-end. Excluded from e2e/e2e-lite; run
# this only when templates change (or via the templates-validate GH
# workflow). Pulls ~20 images, can take 30+ min depending on network.
e2e-templates:
	cd e2e && npm ci && npx playwright install chromium && E2E_TEMPLATES=1 npx playwright test
