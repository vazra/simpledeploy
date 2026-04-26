.PHONY: build build-go test clean ui-build dev api ui api-non-hmr e2e e2e-lite e2e-headed e2e-report e2e-templates e2e-mirror e2e-lite-mirror hooks-install mirror-images-list docker-build dev-docker dev-docker-down dev-docker-rebuild

DEV_GOARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')

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

# Run simpledeploy in a container locally (Docker Desktop friendly).
# Endpoint-only apps (no `ports:` published) are reachable on this path because
# the container joins simpledeploy-public. Stop any native simpledeploy first.
dev-docker: ui-build
	@if lsof -iTCP:443 -sTCP:LISTEN -nP 2>/dev/null | grep -q simpledep; then \
	  echo "error: a native simpledeploy is bound to :443. Stop it first."; exit 1; \
	fi
	GOOS=linux GOARCH=$(DEV_GOARCH) CGO_ENABLED=0 \
	  go build -ldflags="$(LDFLAGS)" -o simpledeploy ./cmd/simpledeploy
	docker compose -f deploy/docker-compose.dev.yml up --build -d
	@echo ""
	@echo "simpledeploy-dev is up. Manage UI: https://localhost:8500/"
	@echo "Logs: docker compose -f deploy/docker-compose.dev.yml logs -f"
	@echo "Stop: make dev-docker-down"

dev-docker-down:
	docker compose -f deploy/docker-compose.dev.yml down
	rm -f simpledeploy

dev-docker-rebuild:
	GOOS=linux GOARCH=$(DEV_GOARCH) CGO_ENABLED=0 \
	  go build -ldflags="$(LDFLAGS)" -o simpledeploy ./cmd/simpledeploy
	docker compose -f deploy/docker-compose.dev.yml up -d --build simpledeploy

clean:
	rm -rf bin/ cmd/simpledeploy/ui_dist

# Install git hooks (pre-push runs vet, build, short tests).
# Bypass hooks with `git push --no-verify` or `SIMPLEDEPLOY_SKIP_HOOKS=1 git push`.
hooks-install:
	git config core.hooksPath .githooks
	@echo "git hooks installed (.githooks)"

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

# E2E with GHCR image mirror (no Docker Hub rate limits). Set
# SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX to override the default
# ghcr.io/vazra/simpledeploy-mirror/.
e2e-mirror:
	cd e2e && npm ci && npx playwright install chromium && E2E_USE_MIRROR=1 npx playwright test

e2e-lite-mirror:
	cd e2e && npm ci && npx playwright install chromium && E2E_LITE=1 E2E_USE_MIRROR=1 npx playwright test

# Print the image list the mirror workflow will push to GHCR.
mirror-images-list:
	@node e2e/scripts/list-mirror-images.mjs

# Build multi-arch Docker image via goreleaser snapshot (no publish). Local smoke test.
docker-build:
	goreleaser release --snapshot --clean --skip=publish
