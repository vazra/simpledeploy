# SimpleDeploy E2E Testing Suite - Design Spec

## Overview

Playwright-based E2E test suite for release quality assurance. Tests build the SimpleDeploy binary, start a server with test config, and exercise every feature through the UI in a single serial session that mimics real user behavior.

## Goals

- Cover all ~95 API endpoints through UI flows
- Catch regressions before releases
- Test real Docker Compose deployments, not mocks
- Single command to run: `make e2e`

## Architecture

```
e2e/
  playwright.config.js          # serial, single worker, chromium
  package.json                  # playwright + dependencies
  global-setup.js               # build binary, start server, wait for health
  global-teardown.js            # kill server, cleanup temp dirs
  fixtures/
    compose-nginx.yml           # single nginx service
    compose-multi.yml           # nginx + redis (multi-service, scale testing)
    compose-postgres.yml        # postgres (backup detection testing)
  helpers/
    server.js                   # server lifecycle: build, start, stop, config gen
    auth.js                     # login(page, user, pass), getAuthenticatedPage()
    api.js                      # direct API calls for setup/teardown/verification
  tests/
    01-setup.spec.js
    02-login.spec.js
    03-deploy.spec.js
    04-dashboard.spec.js
    05-app-overview.spec.js
    06-app-actions.spec.js
    07-app-config.spec.js
    08-app-endpoints.spec.js
    09-app-logs.spec.js
    10-app-metrics.spec.js
    11-app-versions.spec.js
    12-backups.spec.js
    13-alerts.spec.js
    14-users.spec.js
    15-registries.spec.js
    16-docker.spec.js
    17-system.spec.js
    18-profile.spec.js
    19-cleanup.spec.js
```

## Server Lifecycle

### global-setup.js
1. Run `make build` from project root
2. Create temp dirs: `data_dir`, `apps_dir`
3. Generate test config YAML:
   - `listen_addr: ":0"` (unused, TLS off)
   - `management_port: <random available port>`
   - `data_dir: <temp>`
   - `apps_dir: <temp>`
   - `tls.mode: "off"`
   - `master_secret: "test-secret-key-for-e2e-testing!!"` (32 bytes)
   - `log_buffer_size: 100`
4. Spawn `./simpledeploy serve --config <test-config.yml>`
5. Poll `GET http://localhost:<port>/api/health` until 200 (timeout 30s)
6. Store port + PID in env for tests

### global-teardown.js
1. Kill server process (SIGTERM, fallback SIGKILL)
2. Remove temp dirs
3. Remove test config file

## Test Fixtures

### compose-nginx.yml
```yaml
services:
  web:
    image: nginx:alpine
    ports:
      - "8081:80"
    labels:
      simpledeploy.endpoints.0.domain: "nginx-test.local"
      simpledeploy.endpoints.0.port: "80"
      simpledeploy.endpoints.0.tls: "off"
```

### compose-multi.yml
```yaml
services:
  web:
    image: nginx:alpine
    ports:
      - "8082:80"
    labels:
      simpledeploy.endpoints.0.domain: "multi-test.local"
      simpledeploy.endpoints.0.port: "80"
      simpledeploy.endpoints.0.tls: "off"
  cache:
    image: redis:alpine
```

### compose-postgres.yml
```yaml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: testpass
      POSTGRES_DB: testdb
    volumes:
      - pgdata:/var/lib/postgresql/data
    labels:
      simpledeploy.endpoints.0.port: "5432"
      simpledeploy.endpoints.0.tls: "off"
volumes:
  pgdata:
```

## Test Specs - Detailed Coverage

### 01-setup.spec.js - Initial Setup
- Visit `/` redirects to login page
- Login page shows setup mode (no users exist)
- Setup form: username, password, display name, email
- Validation: short password rejected, empty username rejected
- Successful setup creates super_admin, redirects to dashboard

### 02-login.spec.js - Authentication
- Login with correct credentials succeeds, redirects to dashboard
- Login with wrong password shows error
- Logout clears session, redirects to login
- Accessing protected route without session redirects to login
- Re-login after logout works
- Rate limiting: 10+ rapid failed logins show lockout message

### 03-deploy.spec.js - App Deployment
- Open deploy wizard from dashboard
- Deploy nginx app: paste compose YAML, set app name, deploy
- Verify deploy logs stream in real-time
- Verify app appears on dashboard as "running"
- Deploy multi-service app (nginx + redis)
- Deploy postgres app
- Validation: invalid YAML rejected, empty name rejected, duplicate name rejected
- All 3 apps visible on dashboard

### 04-dashboard.spec.js - Dashboard
- All 3 app cards visible with correct status badges
- System metrics section renders (CPU, memory charts)
- Filter apps by status (running)
- App cards show endpoint info
- Click app card navigates to app detail

### 05-app-overview.spec.js - App Detail Overview
- Navigate to nginx app detail
- Overview tab shows: status, services list, endpoints, last deploy time
- Services list shows container status (running)
- Multi-service app shows both nginx and redis services

### 06-app-actions.spec.js - App Actions
- Stop nginx app, verify status changes to "stopped"
- Start nginx app, verify status changes to "running"
- Restart nginx app, verify stays "running"
- Scale redis service in multi app to 2 replicas
- Verify service count updates
- Pull latest images for nginx app

### 07-app-config.spec.js - Configuration
- View compose file in config tab
- Edit compose via YAML editor, save
- View environment variables
- Add new env var, save, verify persisted
- Edit existing env var, save
- Delete env var, save

### 08-app-endpoints.spec.js - Endpoints & Access
- View current endpoints in settings tab
- Edit endpoint domain
- Change TLS mode
- Add IP allowlist entry
- Remove IP allowlist entry

### 09-app-logs.spec.js - Logs
- Open deploy logs, verify log content visible
- Verify WebSocket connection (logs stream indicator)
- Container logs tab shows output

### 10-app-metrics.spec.js - Metrics
- Metrics tab renders charts
- Switch time range (1h, 6h, 24h)
- System metrics page shows CPU/memory/disk charts
- App metrics show per-container data

### 11-app-versions.spec.js - Deploy History
- Versions list shows deploy entries
- Each entry has timestamp, status
- Trigger rollback to previous version
- Verify rollback creates new deploy event
- Delete old version entry

### 12-backups.spec.js - Backups
- Navigate to postgres app backups tab
- Auto-detect shows postgres strategy available
- Create backup config (local target, manual schedule)
- Trigger manual backup
- Verify backup run appears with success status
- Navigate to global backups page, verify summary
- Delete backup config

### 13-alerts.spec.js - Alerts & Webhooks
- Navigate to alerts page
- Create webhook (custom HTTP type, test URL)
- Test webhook delivery, verify success indicator
- Create alert rule (CPU > 90%, link to webhook)
- Edit alert rule threshold
- View alert history (may be empty)
- Delete alert rule
- Delete webhook

### 14-users.spec.js - User Management
- Navigate to users page
- Create new user (viewer role)
- Verify user appears in list
- Edit user display name
- Grant app access to user for nginx app
- Revoke app access
- Create API key, verify key displayed
- Delete API key
- Delete user

### 15-registries.spec.js - Registries
- Navigate to registries page
- Add new registry (Docker Hub, with test creds)
- Verify registry appears in list
- Edit registry URL
- Delete registry

### 16-docker.spec.js - Docker Management
- Navigate to Docker page
- View Docker system info
- View disk usage breakdown
- Images list shows pulled images (nginx, redis, postgres)
- Networks list renders
- Volumes list renders
- Prune stopped containers (confirm dialog)

### 17-system.spec.js - System Administration
- Navigate to system page
- System info displays (version, OS, uptime)
- Storage breakdown renders
- View audit log (shows previous actions)
- Prune old metrics (select tier + days)
- Vacuum database
- Download database backup
- View/edit audit config

### 18-profile.spec.js - Profile
- Navigate to profile page
- Verify current user info displayed
- Update display name, verify saved
- Change password (old + new), verify can re-login with new password
- Toggle theme (dark/light), verify persists

### 19-cleanup.spec.js - Cleanup & Teardown
- Remove postgres app, confirm dialog
- Remove multi-service app
- Remove nginx app
- Dashboard shows no apps
- Verify Docker projects cleaned up

## Playwright Config

```javascript
{
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['html', { open: 'never' }], ['list']],
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env.SIMPLEDEPLOY_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
  globalSetup: './global-setup.js',
  globalTeardown: './global-teardown.js',
}
```

## Makefile Integration

```makefile
e2e: build
	cd e2e && npm ci && npx playwright install chromium && npx playwright test

e2e-headed: build
	cd e2e && npm ci && npx playwright install chromium && npx playwright test --headed
```

## Key Helpers

### auth.js
- `login(page, username, password)` - fill form, submit, wait for dashboard
- `logout(page)` - click logout, wait for login page
- `loginAsAdmin(page)` - shorthand with default test admin creds

### api.js
- `apiRequest(method, path, body)` - direct fetch to server (bypasses UI)
- `waitForAppStatus(slug, status)` - poll until app reaches desired status
- `getAppServices(slug)` - fetch running services for verification

### server.js
- `buildBinary()` - run make build
- `startServer(config)` - spawn process, return { port, pid, cleanup }
- `waitForHealth(port, timeout)` - poll health endpoint
- `stopServer(pid)` - graceful shutdown

## Error Handling & Debugging

- Screenshots on failure (automatic)
- Traces on failure (viewable with `npx playwright show-trace`)
- Video recording on failure
- Console log capture
- Server stdout/stderr captured to `e2e/server.log`

## CI Considerations

- Requires: Docker daemon, Go toolchain, Node.js
- Estimated runtime: 3-5 minutes (mostly Docker pull times)
- Exit code reflects test results
- HTML report generated at `e2e/playwright-report/`
