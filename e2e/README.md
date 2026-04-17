# E2E Testing

Browser-based test suite using Playwright. Builds the SimpleDeploy binary, starts a real server with Docker, deploys actual compose apps, and exercises every UI flow.

## Quick Start

```bash
# from project root
make e2e          # headless
make e2e-headed   # visible browser
make e2e-report   # open HTML report from last run
```

Requires: Docker running, Go, Node.js.

## How It Works

1. `global-setup.js` runs `make build`, starts the binary with a temp config (TLS off, temp data/apps dirs, random port)
2. Tests run serially in numbered order against this server
3. State builds up across tests: setup creates admin -> deploy creates apps -> later tests use those apps
4. `global-teardown.js` kills the server and cleans up temp dirs

Each test gets a fresh browser context (isolated cookies) but shares the server and its state (apps, users, etc).

## Test Files

| File | What it covers |
|------|---------------|
| `01-setup.spec.js` | First-time admin account creation, validation |
| `02-login.spec.js` | Login, logout, wrong password, session redirect |
| `03-deploy.spec.js` | Deploy 3 apps via wizard, YAML validation, redeploy |
| `04-dashboard.spec.js` | App cards, running status, metrics, search, nav |
| `05-app-overview.spec.js` | App detail page, services list, tab navigation |
| `06-app-actions.spec.js` | Stop, start, restart, pull & update, scale |
| `07-app-config.spec.js` | Compose editor, environment variables |
| `08-app-endpoints.spec.js` | Endpoints, TLS badges, advanced settings |
| `09-app-logs.spec.js` | Log viewer, controls, deploy events |
| `10-app-metrics.spec.js` | Chart rendering, time range switching |
| `11-app-versions.spec.js` | Deploy history, version entries, redeploy + image verification |
| `12-backups.spec.js` | Backup wizard UI + **functional roundtrip**: postgres insert→backup→drop→restore→verify, volume archive inspection, retention pruning, hooks, upload-restore, checksum |
| `13-alerts.spec.js` | Webhooks CRUD + **functional dispatch**: webhook receiver + firing/resolved transitions + disabled rule + backup alerts |
| `13b-webhook-formats.spec.js` | Payload schema per integration type: Slack, Telegram, Discord, custom default, template override |
| `14-users.spec.js` | User CRUD, roles, API keys, RBAC (viewer access, super_admin guards, invalid key) |
| `15-registries.spec.js` | Registry add/delete (UI only; see `29b` for functional pull+auth) |
| `16-docker.spec.js` | Docker info, images, networks, volumes, prune |
| `17-system.spec.js` | System info, maintenance, DB backup download + sqlite validation |
| `18-profile.spec.js` | Profile edit, password change, theme toggle |
| `20-security-validation.spec.js` | Compose security (privileged, host network/pid/ipc, caps, volumes), app name validation |
| `21-user-validation.spec.js` | Duplicate username/email, short password, role guards, lockout |
| `22-env-and-endpoints.spec.js` | Env var CRUD, endpoint management, IP access allowlist validation |
| `23-rollback-and-restore.spec.js` | Deploy versions, rollback (verify image tag), backup restore |
| `24-multi-user-isolation.spec.js` | Viewer role isolation, per-app access, API key auth |
| `25-edge-cases.spec.js` | Audit log content, alert/webhook editing, dashboard filters, system prune |
| `26-tls-ssl.spec.js` | **Isolated server** with `tls.mode=local`: self-signed cert serving, custom cert upload, endpoint TLS persistence |
| `27-backup-s3.spec.js` | **MinIO fixture**: S3 roundtrip, checksum, presigned URL, retention, bogus-creds failure |
| `28-db-strategies.spec.js` | MySQL, MongoDB, Redis, SQLite strategy detection + full roundtrip against real DB containers |
| `29a-ratelimit.spec.js` | **Isolated server** with tight limits: burst→429, window recovery |
| `29b-private-registry.spec.js` | **registry:2 fixture**: push private image, register creds via API, deploy succeeds; deploy fails when creds removed |
| `99-cleanup.spec.js` | Remove all apps, verify clean dashboard |

## Test Fixtures

Compose files in `fixtures/` are deployed during tests:

Core apps (used by most specs):
- **compose-nginx.yml** - single nginx service with endpoint labels
- **compose-multi.yml** - nginx + redis (multi-service, scale testing)
- **compose-postgres.yml** - postgres with volume (backup detection + roundtrip)

DB strategy fixtures (used by `28-db-strategies.spec.js`, deployed per-describe):
- **compose-mysql.yml** - mysql:8 with root+user accounts
- **compose-mongo.yml** - mongo:7 with root auth
- **compose-redis.yml** - redis:7-alpine with volume at /data
- **compose-sqlite.yml** - nginx with `simpledeploy.backup.strategy=sqlite` label and a seeded sqlite DB

Security fixture compose files in `fixtures/security/` are used for validation tests (never deployed):
- **compose-privileged.yml**, **compose-host-network.yml**, **compose-host-pid.yml**, **compose-host-ipc.yml** - dangerous runtime options
- **compose-dangerous-caps.yml** - SYS_ADMIN capability
- **compose-dangerous-volumes.yml** - docker socket mount
- **compose-path-traversal.yml** - path traversal in volume source

## External-system fixtures (containers managed by tests, not simpledeploy)

Some specs spin up their own fixture containers via docker CLI and tear them down in `afterAll`. They're NOT managed by simpledeploy.

- **MinIO** (`helpers/minio.js`, used by `27-backup-s3.spec.js`) — runs `minio/minio:latest` on a random localhost port, creates a bucket via `minio/mc`. Use for testing the S3 target without real AWS credentials. Returns `{endpoint, accessKey, secretKey, bucket, mc(), listObjects(), stop()}`.
- **Registry** (`helpers/registry.js`, used by `29b-private-registry.spec.js`) — runs `registry:2` with htpasswd auth on a random localhost port. Helper `pushImage(reg, from, toName)` handles pull→tag→login→push→logout. `startRegistry()` returns `{host, user, pass, stop()}`. Docker Desktop treats `127.0.0.0/8` as insecure by default; Linux Docker does too, so plain HTTP to `localhost:<port>` works.

## Helpers

### `helpers/auth.js`

```js
import { loginAsAdmin, login, logout, getState, TEST_ADMIN } from '../helpers/auth.js';

// login as the test admin
await loginAsAdmin(page);

// login as specific user
await login(page, 'username', 'password');

// get server URL and config
const { baseURL, port } = getState();
```

### `helpers/api.js`

Direct API calls (no browser), useful for setup/teardown:

```js
import { apiRequest, apiLogin, waitForAppStatus } from '../helpers/api.js';

await apiLogin('admin', 'password');
await apiRequest('DELETE', '/api/apps/my-app');
await waitForAppStatus('my-app', 'running', 60_000);
```

### `helpers/server.js`

Server lifecycle. `startServer` is exported so individual specs can spin up their own isolated server (used by `26-tls-ssl`, `29a-ratelimit`, `29b-private-registry`).

- `buildBinary()` - runs `make build` (used only by global-setup)
- `getBinaryPath()` - returns the already-built binary path without rebuilding
- `startServer(binPath, overrides?)` - starts on random ports, waits for health. Overrides: `{proxyPort, dataDir, appsDir, tlsMode, ratelimit}`. `ratelimit` is `{requests, window, burst, by}` and applies to the auth/login rate limiter.
- `stopServer(server)` - kills process, cleans temp dirs

### `helpers/docker.js`

Container-level helpers. Filter by `com.docker.compose.project=simpledeploy-<slug>` and `com.docker.compose.service=<name>` — not a nonexistent `simpledeploy.app` label.

- `findServiceContainer(slug, service)` — returns full container name (`simpledeploy-<slug>-<service>-1`)
- `listAppContainers(slug)`, `countServiceReplicas(slug, service)`
- `containerRunning(name)`, `containerImage(name)`, `dockerInspect(name)`
- `waitForContainerState(name, running, timeoutMs)`
- `waitForHealthy(name, healthCmd, timeoutMs)` — polls `docker exec` until exit 0
- `psql(container, user, db, sql)` — passes SQL as a single argv element so there's no shell quoting

### `helpers/dbclients.js`

Mirror of `psql()` for the other DB strategies, used by `28-db-strategies.spec.js`:
- `mysqlExec(container, password, db, sql)` — `docker exec ... mysql -u root -p<pw>`
- `mongoEval(container, user, password, js)` — `docker exec ... mongosh --authenticationDatabase admin --username ... --eval <js>`
- `redisCmd(container, ...args)` — `docker exec ... redis-cli <args>`
- `sqlite3Eval(container, dbPath, sql)` — `docker exec ... sqlite3 <path> <sql>`

All log to `/tmp/e2e-dbclients-trace.log` for debugging. None use `sh -c`.

### `helpers/proxy.js`

Reverse-proxy HTTP helpers. `fetchViaProxy(host, path, opts)` wraps curl under the hood because Node's `fetch` silently drops the `Host` header.

- `fetchViaProxy(host, path, opts)` — returns `{status, ok, text(), json()}`
- `curlHTTPS(host, port, path, opts)` — with `-k` (accept self-signed), for `26-tls-ssl`
- `openSSLGetCert(host, port)` — parses `openssl s_client` output to a `{subject, issuer, notBefore, notAfter}` object

### `helpers/webhook.js`

Local HTTP listener for testing alert webhook dispatch. Server runs `SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS=1` so the dispatcher accepts `127.0.0.1`.

```js
const receiver = await startWebhookReceiver();
// receiver.url → http://127.0.0.1:<port>
// receiver.received → array of {method, path, headers, body, at}
// receiver.waitFor(predicate, timeoutMs) → resolves when a matching POST lands
// receiver.clear(), receiver.stop()
```

### `helpers/db.js`

Direct sqlite queries against simpledeploy's DB (different from `dbclients.js` which targets app containers).

- `sqliteQuery(sql)`, `sqliteExec(sql)`, `tableCount(table)`
- `getAppId(slug)` — resolves an app slug to the FK used in metrics/alert tables
- `insertMetricPoint({appSlug, cpu, memoryMb, tsSec, tier})` — injects synthetic metrics for deterministic alert evaluation
- `injectHighCPUWindow(appSlug)` — shortcut that inserts 10 high-CPU points spread over the last minute

### `helpers/minio.js`

MinIO fixture for S3 target tests. See "External-system fixtures" above.

### `helpers/registry.js`

Private registry fixture (registry:2 + htpasswd). See "External-system fixtures" above.

## Writing New Tests

### Add a new spec file

Create `e2e/tests/NN-feature.spec.js` with a number that places it in the right execution order. Tests that need deployed apps should come after `03-deploy.spec.js`.

### Basic template

```js
import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('My Feature', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('does something', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/my-page`);
    await expect(page.getByText('Expected Content')).toBeVisible();
  });
});
```

### Selector guidelines

**SPA uses hash routing.** URLs look like `http://localhost:PORT/#/apps/my-app`.

**Prefer role/text selectors over CSS:**
```js
// good
page.getByRole('button', { name: 'Deploy App' })
page.getByPlaceholder('my-app')
page.getByText('running')

// avoid
page.locator('.btn-primary')
```

**Scope to avoid sidebar matches.** Text like usernames and nav labels appear in both sidebar and main content:
```js
// bad - might match sidebar
page.getByText('e2eadmin')

// good - scoped to main
page.locator('main').getByText('e2eadmin').first()
```

**Scope modal interactions to dialog:**
```js
const dialog = page.getByRole('dialog');
await dialog.getByPlaceholder('name').fill('test');
await dialog.getByRole('button', { name: 'Create' }).click();
```

**Use `.first()` when multiple matches possible:**
```js
await expect(page.getByText('running').first()).toBeVisible();
```

**Use `{ exact: true }` to prevent substring matching:**
```js
// "e.g. jane" would also match "e.g. Jane Doe" without exact
await dialog.getByPlaceholder('e.g. jane', { exact: true }).fill('testuser');
```

### Waiting for state changes

```js
// wait for sidebar to confirm login succeeded
await page.waitForSelector('aside', { timeout: 15_000 });

// wait for app action to complete (action modal shows Close button)
const closeBtn = page.getByRole('button', { name: /close/i });
await expect(closeBtn).toBeVisible({ timeout: 60_000 });

// wait for toast notification
await expect(page.locator('[role="alert"]')).toBeVisible({ timeout: 5_000 });
```

## Server Configuration

The test server starts with these settings (see `helpers/server.js`):

| Setting | Value | Why |
|---------|-------|-----|
| `tls.mode` | `"off"` | No certs needed, avoids HTTPS complexity |
| `management_port` | random | Prevents port conflicts |
| `master_secret` | fixed string | Deterministic JWT/encryption for tests |
| `ratelimit.requests` | 10000 | Prevents login failures across 91 tests |
| `data_dir` / `apps_dir` | temp dirs | Clean state per run, auto-cleaned |
| `log_buffer_size` | 100 | Smaller buffer for test performance |

## Debugging Failures

**View the HTML report:**
```bash
make e2e-report
```

**View a trace (recorded on failure):**
```bash
npx playwright show-trace e2e/test-results/<test-dir>/trace.zip
```

**Run with visible browser:**
```bash
make e2e-headed
```

**Run with Playwright Inspector (step through):**
```bash
cd e2e && npx playwright test --debug
```

**Check server logs from failed run:**
The server log path is in `.e2e-state.json` (auto-cleaned, but survives if teardown fails).

## Gotchas (read before writing functional tests)

Notes accumulated while wiring the functional E2E layer. These will bite you if you miss them.

### Docker container naming

Compose projects are prefixed `simpledeploy-<slug>`. Containers use the compose convention `<project>-<service>-<replica>`. Example:

| App slug        | Project                        | Container (first replica) |
|-----------------|--------------------------------|---------------------------|
| `e2e-nginx`     | `simpledeploy-e2e-nginx`       | `simpledeploy-e2e-nginx-web-1` |
| `e2e-postgres`  | `simpledeploy-e2e-postgres`    | `simpledeploy-e2e-postgres-db-1` |

There is no `simpledeploy.app=<slug>` label on containers. Filter by `com.docker.compose.project=simpledeploy-<slug>` and `com.docker.compose.service=<name>` instead. The `helpers/docker.js` helpers already do this; don't roll your own.

When calling `strategy.Detect()` in the backup package, pass the compose project name (`"simpledeploy-" + app.Name`) as the compose project name so the returned `ContainerName` matches reality.

### `store.App` JSON serialization is PascalCase

`store.App` has no `json:"..."` tags, so fields serialize as `Name`, `Slug`, `Status`, etc. — not the conventional lowercase. Handlers that embed the struct inherit this. Tests checking `res.data.slug === 'foo'` will silently fail. Use `res.data.Slug || res.data.slug` defensively, or fix the JSON tags.

### Node's `fetch` silently drops the `Host` header

WHATWG spec lists `Host` as a forbidden header. Node's `undici`-based `fetch` silently strips it, sending `localhost:<port>` instead. Routing through Caddy by `Host` header is therefore broken via `fetch`. `helpers/proxy.js` `fetchViaProxy` uses `curl` under the hood to work around this.

### Caddy returns empty `200` for unrouted hosts

`automatic_https.disable: true` (tls.mode=off) makes Caddy emit an empty 200 for hosts it has no route for. Don't assert `status !== 200` to detect a missing route; check the response body instead.

### `psql` via `docker exec` — pass SQL as an argv element

Shell quoting through `sh -c` is fragile when SQL contains single quotes, pipes, dollar signs, etc. The `psql` helper in `helpers/docker.js` spawns docker directly with the SQL as its own argv element:

```js
execFileSync('docker', ['exec', container, 'psql', '-U', user, '-d', db, '-t', '-A', '-c', sql], { ... });
```

No shell = no quoting hazards.

### Postgres backup requires `-d <database>`

`pg_dump -U postgres` without `-d` defaults to the database named after the user, which on the official postgres image is the empty `postgres` DB (not `$POSTGRES_DB`). The strategy reads `$POSTGRES_DB` and `$POSTGRES_USER` from the container env at dump time via `sh -c`.

### Volume strategy vs. live containers

Backing up `/var/lib/postgresql/data` while postgres is running captures a crash-consistent snapshot. **Restoring** over that live directory is racy — postgres has files open and the extraction may not overwrite them. Use `pre_hooks: [stop]` + `post_hooks: [start]` for restore flows, or verify backups by inspecting the tar.gz archive rather than doing a round-trip.

### `run.file_path` for the local target is just the filename

`local.LocalTarget.Upload` returns the basename. The download handler resolves it against `<dataDir>/backups/`. If you add new code paths that open the backup file, do the same resolution — `os.Open(run.FilePath)` will look in CWD and fail.

### Scaling a service with a fixed host port fails

Compose rejects `docker compose scale web=2` when `web` has `ports: ["8092:80"]` — the second replica can't bind the same host port. Scale a service without port mappings (e.g. the cache/redis side of `e2e-multi`).

### Reconciler uses the request context for async work

`handleEndpoints` fires `go s.reconciler.Reconcile(r.Context())`. The request context is cancelled as soon as the response is sent, so the background Reconcile aborts with `context canceled`. Known foot-gun — use `context.Background()` when you spawn a goroutine that outlives the request.

### Async deploy returns 202 before the container is up

`POST /api/apps/deploy` returns 202 once the goroutine is spawned. Wait for `GET /api/apps/:slug` to return `Status: "running"` before asserting anything about containers. Tests that poll immediately after 202 will see a missing container.

## Functional test coverage

The E2E suite mixes two layers:

1. **UI-workflow tests** — click-through with Playwright (tests/01–18, 22–25).
2. **Functional assertions** — injected into the same specs as `describe.configure({ mode: 'serial' })` blocks. These use docker exec, direct sqlite queries, and curl-via-Caddy to verify the system actually did what the UI said it did.

Examples of functional checks:
- `docker inspect` after stop/start/restart to confirm container state transitions.
- `psql` insert → backup → drop → restore → `psql` select, verifying data roundtrip.
- `tar -tzf` on the backup archive to assert the expected files are captured.
- SHA-256 of a downloaded backup matches the stored `run.checksum`.
- Local HTTP receiver (`helpers/webhook.js`) asserts alert webhook payloads.
- Direct `metrics` table inserts (`helpers/db.js`) drive the alert evaluator deterministically instead of waiting for real CPU load.

### Adding an external-system test

Don't rely on real external services (S3, Telegram, Discord, Let's Encrypt). Stand up a local fixture:
- S3: run MinIO in a docker container, point the backup target at it.
- Webhooks: use `startWebhookReceiver()` from `helpers/webhook.js`, pass its `url` as the webhook URL.
- Let's Encrypt is not testable hermetically; leave `tls.mode=auto` to integration tests against a real domain.

## Known Limitations

- Tests cannot run individually. They depend on prior test state (setup -> deploy -> actions).
- Docker image pulls on first run can be slow (~1-2min for nginx/redis/postgres).
- The backup trigger test accepts any completion status (success or failed) since pg_dump may fail if postgres hasn't fully initialized.
- Svelte 5 collapsible sections sometimes don't respond to Playwright's click. One test works around this.

## CI Integration

```yaml
# GitHub Actions example
- name: E2E Tests
  run: make e2e
  env:
    DOCKER_HOST: unix:///var/run/docker.sock
```

Requires a runner with Docker. The test suite is self-contained: builds the binary, manages its own server, cleans up after itself.
