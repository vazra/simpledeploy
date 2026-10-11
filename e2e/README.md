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

Every test must be able to fail when the feature it names breaks (see [Test design rules](#test-design-rules)). Behavior already pinned by Go tests or vitests is not repeated here; E2E covers what only the full stack can show.

| File | What it covers |
|------|---------------|
| `01-setup.spec.js` | First-run setup creates the admin (setup status flips, `/api/me` profile) |
| `02-login.spec.js` | Login form (the only spec that uses it), wrong password 401, guarded routes, logout revokes the session server-side |
| `03-deploy.spec.js` | Deploys e2e-nginx, e2e-multi, e2e-postgres via the wizard; invalid YAML blocked; name collision suggestion; containers, labels and proxy routing |
| `03c-community-recipes.spec.js` | Recipe catalog from a local fixture server (isolated server): browse, import into the wizard, popularity count |
| `04-dashboard.spec.js` | App cards and status badges match the API, host metrics reach the UI, search, card navigation |
| `06-app-actions.spec.js` | Stop/start/restart/pull/scale through the UI, each verified in Docker and through the proxy |
| `07-app-config.spec.js` | Edit the image in the compose editor, redeploy, container runs the new image |
| `08-app-endpoints.spec.js` | Add an endpoint in Settings, TLS badge, added/removed/multiple domains route through the proxy |
| `09-app-logs.spec.js` | A request through the proxy shows up in the live log viewer; events tab matches the API |
| `10-app-metrics.spec.js` | Range switch returns data; request metrics grow with real traffic |
| `11-app-versions.spec.js` | Version history in the UI matches the API; compose download equals the stored version |
| `12-backups.spec.js` | Backup wizard saves the config; postgres backup, drop, restore roundtrip; volume archive contents; pre/post hooks run in the app container; upload-restore with checksum; tab refreshes live |
| `13-alerts.spec.js` | Webhook create + Send Test to a local receiver; rule form keeps its app; CPU alert fires, resolves, no double-fire, disabled rule silent; backup alerts name and scope the right app |
| `14-users.spec.js` | User and API key lifecycle through the UI (checked via API); RBAC: viewer and manage limits with exact status codes; deleting a user revokes their key |
| `15-registries.spec.js` | Add/delete a registry; password never returned (see `29b` for real pulls) |
| `16-docker.spec.js` | Docker page shows the daemon's real version, images, networks, volumes |
| `17-system.spec.js` | System info matches the API, VACUUM, DB download is a real SQLite file, process logs, compact backup strips metrics |
| `18-profile.spec.js` | Display name and password change (old password rejected, restored in finally) |
| `18a-realtime.spec.js` | Users page updates over the realtime socket without a reload |
| `20-activity.spec.js` | Activity rows for real actions, failed deploy shows its error inline, env change summary, purge |
| `20-export-import.spec.js` | Export, import as a clone that reaches running, overwrite import with preview |
| `20-security-validation.spec.js` | Compose security wiring (violations reported, nothing written), validate-compose matches deploy, app name rules |
| `21-user-validation.spec.js` | Duplicate username/email, short password, own-role and self-delete guards change nothing |
| `22-env-and-endpoints.spec.js` | Env round trip; IP allowlist enforced by the proxy (deny, then allow, then cleared) |
| `23-rollback-and-restore.spec.js` | Rollback switches the running image and routes traffic; roll forward again |
| `24-multi-user-isolation.spec.js` | Viewer session cannot mutate even a granted app |
| `25-edge-cases.spec.js` | Login is recorded in the activity log; metric prune deletes only old rows |
| `26-tls-ssl.spec.js` | **Isolated server**, `tls.mode=local`: internal-CA cert on one endpoint, uploaded custom cert on another |
| `26a-config-recovery.spec.js` | **Isolated server**: DB wiped, restart rebuilds users, keys, webhooks, registries and per-app config from the config files |
| `26b-gitsync.spec.js` | **Isolated server + bare git repo** (`E2E_GITSYNC=1`, on in CI): dashboard changes pushed, remote changes applied, conflict while the remote is offline |
| `27-backup-s3.spec.js` | **MinIO fixture**: S3 roundtrip, presigned download checksum, bad credentials, retention prunes objects and runs |
| `28-db-strategies.spec.js` | MySQL, MongoDB, Redis, SQLite backup/restore roundtrips against real containers |
| `29a-ratelimit.spec.js` | **Isolated server**: exact allow/deny sequence, `Retry-After`, per-key buckets, recovery after the window |
| `29b-private-registry.spec.js` | **registry:2.8.3 fixture**: deploy from a private registry; removing creds fails with an auth error |
| `30-app-templates-ui.spec.js` | Every app template validates (validate-compose and `docker compose config`); multi-endpoint templates in Quick test mode |
| `95-archive.spec.js` | Removing an app folder archives the app (containers and route gone); purge |
| `99-cleanup.spec.js` | UI delete removes containers, folder and API entry; API delete of a multi-service app |
| `templates-deploy-all.spec.js` | Opt-in (`E2E_TEMPLATES=1`, Templates workflow): deploys every app template and probes it over HTTP |

Template image availability is checked by `scripts/check-template-images.mjs` in the Templates workflow (on template changes and weekly), not in the per-push suite.

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
- **compose-sqlite.yml** - `gitea/gitea` image (ships sqlite3) with `simpledeploy.backup.strategy=sqlite` and a seeded sqlite DB; no package installs at test time

`fixtures/helper-images.yml` is never deployed: it lists the images helpers run directly (MinIO, registry) so the mirror workflow copies them.

## External-system fixtures (containers managed by tests, not simpledeploy)

Some specs spin up their own fixture containers via docker CLI and tear them down in `afterAll`. They're NOT managed by simpledeploy. With `E2E_USE_MIRROR=1` (CI) their images come from the GHCR mirror (`helpers/images.js`).

- **MinIO** (`helpers/minio.js`, used by `27-backup-s3.spec.js`): runs `pgsty/silo` (maintained drop-in MinIO fork; official `minio/minio` images are gone) pinned to the same tag as the MinIO templates, on a random localhost port, and creates a bucket via the `mc` client bundled in that image. Use for testing the S3 target without real AWS credentials. Returns `{endpoint, accessKey, secretKey, bucket, mc(), listObjects(), stop()}`.
- **Registry** (`helpers/registry.js`, used by `29b-private-registry.spec.js`): runs `registry:2.8.3` with htpasswd auth (the file is copied in with `docker cp`, since macOS temp dirs are not visible to colima) on a random localhost port. Helper `pushImage(reg, from, toName)` handles pull→tag→login→push→logout. `startRegistry()` returns `{host, user, pass, stop()}`. Docker Desktop treats `127.0.0.0/8` as insecure by default; Linux Docker does too, so plain HTTP to `localhost:<port>` works.

## Helpers

### `helpers/auth.js`

```js
import { loginAsAdmin, login, loginViaUI, logout, getState, TEST_ADMIN } from '../helpers/auth.js';

await loginAsAdmin(page);                    // API login (shares the page's cookies), then opens the dashboard
await login(page, 'username', 'password');   // same, as another user
await loginViaUI(page);                      // through the login form: only for specs testing the form (02)
const { baseURL, port } = getState();
```

Logging out revokes every session of that user (token version bump), including the cookie cached by `helpers/api.js`; call `apiLogin` again after a logout.

### `helpers/api.js`

Direct API calls (no browser), useful for setup/teardown:

```js
import { apiRequest, apiLogin, waitForAppStatus, waitForDeployDone } from '../helpers/api.js';

await apiLogin('admin', 'password');
await apiRequest('DELETE', '/api/apps/my-app');
await waitForAppStatus('my-app', 'running', 60_000);
await waitForDeployDone('my-app'); // a redeploy keeps status "running"; this waits for it to finish
```

### `helpers/server.js`

Server lifecycle. The server config is written to `data_dir/server-config.yaml` (`data_dir/config.yml` is SimpleDeploy's own global config file). Specs that need their own server use `helpers/isolated.js` (`26`, `26a`, `29a`, `29b`, `03c`).

- `buildBinary()` - runs `make build` (used only by global-setup)
- `getBinaryPath()` - returns the already-built binary path without rebuilding
- `startServer(binPath, overrides?)` - starts on random ports, waits for health. Overrides: `{proxyPort, dataDir, appsDir, tlsMode, ratelimit, recipesIndexURL}`. `ratelimit` applies to the management API limiter only.
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

None use `sh -c`. Errors include the full stdout/stderr.

### `helpers/proxy.js`

Reverse-proxy HTTP helpers. `fetchViaProxy(host, path, opts)` wraps curl under the hood because Node's `fetch` silently drops the `Host` header.

- `fetchViaProxy(host, path, opts)` — returns `{status, ok, text(), json()}`
- `curlHTTPS(host, port, path, opts)` — with `-k` (accept self-signed), for `26-tls-ssl`
- `openSSLGetCert(host, port)` — parses `openssl s_client` output to a `{subject, issuer, notBefore, notAfter}` object
- `waitForProxyStatus(host, status, {timeoutMs, bodyContains, path, proxyURL})` — polls until the route answers; use instead of sleeping after an endpoint change

### `helpers/webhook.js`

Local HTTP listener for testing alert webhook dispatch. Server runs `SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS=1` so the dispatcher accepts `127.0.0.1`. It also sets `SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1` so the S3 backup specs can use a local MinIO endpoint.

```js
const receiver = await startWebhookReceiver();
// receiver.url → http://127.0.0.1:<port>
// receiver.received → array of {method, path, headers, body, at}
// receiver.waitFor(predicate, timeoutMs) → resolves when a matching POST lands
// receiver.onPath(path), receiver.countWhere(predicate): per-path assertions (probe/control rules)
// receiver.clear(), receiver.stop()
```

### `helpers/db.js`

Direct sqlite queries against simpledeploy's DB (different from `dbclients.js` which targets app containers).

- `sqliteQuery(sql)`, `sqliteExec(sql)`
- `getAppId(slug)` — resolves an app slug to the FK used in metrics/alert tables
- `replaceMetricsAtomic(appSlug, {...})` — swaps an app's metric points in one transaction (deterministic alert input)

### `helpers/metrics.js`

`startMetricRefresher(slug, high)` keeps a synthetic CPU window in place for the alert evaluator (first write throws; later errors surface in waits), `deleteAppMetrics(slug)`, `waitForCollectorCycle()` (lets an in-flight collector write land before the test starts).

### `helpers/backups.js`

`createBackupConfig`, `triggerAndWait`, `pollRunStatus`, `waitForNewRunForConfig`, `deleteAppConfigsFor`, `localBackupPath`, `waitForNextSecond` (backup filenames have 1s resolution).

### `helpers/users.js`

`findUser`, `deleteUserByName`, `waitForDeployEvent` / `latestDeployEventId` (wait for the deploy a 202 started), `waitForAppSettled` (no deploy running, app running, stored compose hash matches the file).

### `helpers/isolated.js`

`startIsolatedServer(opts)` returns `{server, cookie, api}` for a spec-owned server; `waitForAppStatusAt`, `loginAt`, `deployAt`, `stopGracefully` (SIGTERM and wait, so pending config writes flush), `stopIsolatedServer(server, logName)` (keeps the log in `test-results/`).

### `helpers/recipes.js`

Serves a one-recipe community catalog on 127.0.0.1 for `03c`.

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

// wait for a specific toast, then check the server state it claims
await expect(page.getByRole('alert')).toHaveText('Profile updated');
```

## Test design rules

A test earns its place only if it would fail when the feature it names breaks, and only if Go tests or vitests cannot show the same thing (those take seconds; this suite takes minutes).

- Assert the resulting server state (API, Docker, proxy, files), not just a toast or a visible heading. A toast check also needs its exact text: `[role="alert"]` matches error toasts too.
- No conditional assertions: no `if (await x.isVisible())`, `.catch(() => false)` guards, or try/catch that swallows a failure. If an element may be missing, the test is wrong.
- No fixed sleeps when a condition can be polled (`expect.poll`, `waitForResponse`, `waitForProxyStatus`, `waitForAppStatus`, `waitForDeployDone`).
- Exact status codes, never ranges like `< 500` or `[200, 404]`.
- Scope text matches to `main`, a table or a dialog. The activity sidebar is always in the DOM and lists app names, so page-wide `getByText(...).first()` passes on the wrong element.
- Await background work: after a 202, wait for the app to settle before the test ends, or the next spec inherits a redeploy.
- Put cleanup that later specs depend on in `finally`/`afterAll`. Restore anything shared (admin password, `e2e-nginx` env, allowlists).
- A test for unfixed product behavior asserts the correct behavior and fails until the fix lands. Never skip or loosen it to get green.
- Do not test pure rendering or client-side logic here; write a vitest.

## Server Configuration

The test server starts with these settings (see `helpers/server.js`):

| Setting | Value | Why |
|---------|-------|-----|
| `tls.mode` | `"off"` | No certs needed, avoids HTTPS complexity |
| `management_port` | random | Prevents port conflicts |
| `master_secret` | fixed string | Deterministic JWT/encryption for tests |
| `login_ratelimit.requests` | 10000 | Tests log in many times from 127.0.0.1 |
| `data_dir` / `apps_dir` | temp dirs | Clean state per run, auto-cleaned |
| `log_buffer_size` | 100 | Smaller buffer for test performance |

Tick intervals are shortened through env (alert evaluation 2s, metrics flush 1s, status refresh 5s).

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
- Svelte 5 collapsible sections sometimes don't respond to Playwright's click. One test works around this.

## CI Integration

- `.github/workflows/ci.yml` runs `make e2e` on every push to `main` with `E2E_USE_MIRROR=1` (images from the GHCR mirror, no Docker Hub limits) and `E2E_GITSYNC=1`. It uploads `playwright-report/` and `test-results/` (failure traces plus `server.log`, which global teardown copies out before deleting the temp dirs).
- `.github/workflows/templates.yml` runs the template image check and the `templates-deploy-all` matrix when templates or the compose/deployer code change, and weekly.
- Global teardown removes compose stacks left behind by failed specs (only stacks whose folder is under an `sd-e2e-*` temp dir).
