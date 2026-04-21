---
title: Build and test
description: Make targets and test commands for building, running, and verifying SimpleDeploy locally and in CI.
---

## Make targets

| Target | Description |
|--------|-------------|
| `make build` | Full build: UI + Go binary into `bin/simpledeploy` |
| `make build-go` | Go only. Requires `cmd/simpledeploy/ui_dist/` to exist |
| `make ui-build` | Build Svelte UI and copy `dist/` into `cmd/simpledeploy/ui_dist/` |
| `make test` | All Go unit/integration tests (`go test ./...`) |
| `make e2e` | Build binary, start a real server, run Playwright suite headless |
| `make e2e-headed` | Same, with a visible browser window |
| `make e2e-report` | Open the HTML report from the last E2E run |
| `make dev` | Hot-reload: Air for Go API + Vite for UI |
| `make api` | API only with hot-reload (Air) |
| `make ui` | Vite dev server only |
| `make api-non-hmr` | Build + run with `config.dev.yaml`, no reloader |
| `make clean` | Remove `bin/` and `cmd/simpledeploy/ui_dist/` |
| `make hooks-install` | Enable git hooks from `.githooks/` (pre-push: vet + lint + build + short tests + vitest) |

## Git hooks

After cloning, run `make hooks-install` once. This points `core.hooksPath` at `.githooks/` and enables:

- `pre-push`: `go vet ./...`, `golangci-lint run ./...`, `go build ./...`, `go test -short ./...`, and `ui` vitest if `ui/node_modules` exists.

`golangci-lint` is required (mirrors CI). Install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4`. Skip just the lint step with `SIMPLEDEPLOY_SKIP_LINT=1 git push`.

Bypass all hooks with `git push --no-verify` or `SIMPLEDEPLOY_SKIP_HOOKS=1 git push`.

## Go tests

```bash
go test ./...                              # everything
go test ./internal/api/ -v                 # one package
go test ./internal/store/ -run TestUpsert  # one test
go test -race -count=1 ./internal/...      # race detector, no cache
```

Conventions:

- Docker tests skip cleanly when the daemon is unavailable.
- Store tests use a temp DB file per test.
- API tests use `httptest` + a real store.
- Mock Docker client in `internal/docker/mock.go`.
- Mock command runner in `internal/deployer/runner.go`.

## UI unit tests (vitest)

Fast Svelte + JS tests covering pure helpers, stores, components, and routes. Run in ~4 seconds and gate every push via the `pre-push` hook.

```bash
cd ui
npm test              # run once
npm run test:watch    # watch mode
```

Tests live in `ui/src/**/__tests__/*.test.js` and use `@testing-library/svelte` + jsdom. `src/test-mocks/api.js` exposes `makeApiMock()` for stubbing the API module, and `src/routes/__tests__/LayoutStub.svelte` passes through `Layout` children so route-level tests stay focused.

**Any UI change that adds new behavior, fixes a bug, or modifies component logic must include or update a vitest.** See [UI Test Coverage Rule](../../CLAUDE.md#ui-test-coverage-rule) for the full policy.

Skip only for pure visual tweaks, docblock-only edits, or behavior fully covered by E2E where a vitest would duplicate without adding signal.

## E2E tests

```bash
make e2e           # full suite, headless, ~2.5 min
make e2e-headed    # with browser window
make e2e-report    # open last HTML report
```

Requires Docker running, Go, Node.js. The harness builds the binary, starts a real server on a random port with TLS off, and deploys real fixture apps. See [E2E tests](/contributing/e2e-tests/) for adding cases.

Prefer a vitest over a new E2E spec when the logic can be exercised with mocks. Vitests run in seconds; the full E2E suite takes ~20 minutes.

## Where to add tests for each kind of change

| Change | Add tests in |
|--------|--------------|
| New store method | `internal/store/<feature>_test.go` |
| New API endpoint | `internal/api/<handler>_test.go` plus E2E flow |
| New backup strategy | `internal/backup/<name>_test.go` |
| New backup target | `internal/backup/<name>_test.go` |
| New Caddy module | `internal/proxy/<module>_test.go` |
| New UI component | `ui/src/components/__tests__/<Name>.test.js` |
| New UI lib/helper | `ui/src/lib/__tests__/<name>.test.js` |
| New UI route | `ui/src/routes/__tests__/<Name>.test.js` (smoke + key interaction) |
| UI flow (full-stack) | New or extended file in `e2e/tests/` |
| CLI command | `cmd/simpledeploy/*_test.go` if behavior is non-trivial |

## CI

| Job | Trigger | What it does |
|-----|---------|--------------|
| `lint` | push + PR | `golangci-lint` |
| `test` | push + PR | `go test ./...` and `cd ui && npm test` |
| `build` | push + PR | full `make build` |
| `e2e` | push to `main` | Playwright against a real server |

All jobs except E2E must pass before a PR can merge.
