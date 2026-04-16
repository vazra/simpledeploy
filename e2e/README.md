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
| `11-app-versions.spec.js` | Deploy history, version entries |
| `12-backups.spec.js` | Backup wizard, manual trigger, global summary |
| `13-alerts.spec.js` | Webhooks CRUD, alert rules CRUD, history |
| `14-users.spec.js` | User CRUD, roles, API keys |
| `15-registries.spec.js` | Registry add/delete |
| `16-docker.spec.js` | Docker info, images, networks, volumes, prune |
| `17-system.spec.js` | System info, maintenance, audit log, logs |
| `18-profile.spec.js` | Profile edit, password change, theme toggle |
| `20-security-validation.spec.js` | Compose security (privileged, host network/pid/ipc, caps, volumes), app name validation |
| `21-user-validation.spec.js` | Duplicate username/email, short password, role guards, lockout |
| `22-env-and-endpoints.spec.js` | Env var CRUD, endpoint management, IP access allowlist validation |
| `23-rollback-and-restore.spec.js` | Deploy versions, rollback, backup restore |
| `24-multi-user-isolation.spec.js` | Viewer role isolation, per-app access, API key auth |
| `25-edge-cases.spec.js` | Audit log content, alert/webhook editing, dashboard filters, system prune |
| `99-cleanup.spec.js` | Remove all apps, verify clean dashboard |

## Test Fixtures

Three compose files in `fixtures/` are deployed during tests:

- **compose-nginx.yml** - single nginx service with endpoint labels
- **compose-multi.yml** - nginx + redis (multi-service, scale testing)
- **compose-postgres.yml** - postgres with volume (backup detection testing)

Security fixture compose files in `fixtures/security/` are used for validation tests (never deployed):

- **compose-privileged.yml**, **compose-host-network.yml**, **compose-host-pid.yml**, **compose-host-ipc.yml** - dangerous runtime options
- **compose-dangerous-caps.yml** - SYS_ADMIN capability
- **compose-dangerous-volumes.yml** - docker socket mount
- **compose-path-traversal.yml** - path traversal in volume source

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

Server lifecycle (used by global setup/teardown, not directly by tests):

- `buildBinary()` - runs `make build`
- `startServer(binPath)` - starts on random port, waits for health
- `stopServer(server)` - kills process, cleans temp dirs

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
