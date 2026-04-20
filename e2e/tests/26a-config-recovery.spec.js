/**
 * DR: sidecar config recovery after DB wipe.
 *
 * Spawns its own isolated server (fresh data + apps dirs). Does NOT touch the
 * shared server from global-setup. Requires the binary to be built first:
 *   make build-go   # or make build
 *
 * Run standalone:
 *   cd e2e && npx playwright test 26a-config-recovery.spec.js --reporter=list
 */

import { test, expect } from '@playwright/test';
import { rmSync, existsSync, unlinkSync, mkdtempSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import { getBinaryPath, startServer } from '../helpers/server.js';
import { apiRequestAt } from '../helpers/api.js';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ADMIN = { username: 'dr-admin', password: 'DrTestPass123!', displayName: 'DR Admin', email: 'dr@test.local' };

/** Bootstrap admin account via POST /api/setup. */
async function setupAdmin(baseURL) {
  const res = await apiRequestAt(baseURL, 'POST', '/api/setup', ADMIN, null);
  if (!res.ok) throw new Error(`setup failed: ${res.status} ${JSON.stringify(res.data)}`);
  return res;
}

/** Login and return sessionCookie. */
async function login(baseURL) {
  const res = await apiRequestAt(baseURL, 'POST', '/api/auth/login', {
    username: ADMIN.username,
    password: ADMIN.password,
  }, null);
  if (!res.ok) throw new Error(`login failed: ${res.status} ${JSON.stringify(res.data)}`);
  if (!res.setCookie) throw new Error('no session cookie after login');
  return res.setCookie;
}

/** Authenticated request against an isolated server. */
function at(baseURL, cookie) {
  return (method, path, body) => apiRequestAt(baseURL, method, path, body, cookie);
}

/** Minimal nginx compose (no external ports to avoid collision). */
const COMPOSE_NGINX = `services:
  web:
    image: nginx:alpine
`;

/** Stop a server process without deleting its data dirs. */
function stopProc(srv) {
  if (!srv) return;
  try { srv.proc.kill('SIGTERM'); } catch {}
}

/** Delete SQLite DB files from dataDir. */
function wipeDB(dataDir) {
  for (const suffix of ['simpledeploy.db', 'simpledeploy.db-wal', 'simpledeploy.db-shm']) {
    const p = join(dataDir, suffix);
    if (existsSync(p)) unlinkSync(p);
  }
}

/** Wait until GET /api/health returns 200. */
async function waitHealthy(baseURL, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseURL}/api/health`);
      if (res.ok) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`server at ${baseURL} not healthy after ${timeoutMs}ms`);
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

test.describe('DR: sidecar recovery after DB wipe', () => {
  test.setTimeout(120_000);

  let srv1 = null;

  test.afterAll(async () => {
    // Kill any leftover server. Dirs are cleaned below or already cleaned.
    stopProc(srv1);
    if (srv1) {
      for (const dir of [srv1.dataDir, srv1.appsDir]) {
        try { rmSync(dir, { recursive: true, force: true }); } catch {}
      }
    }
  });

  test('DB wipe and restart recovers app, webhook, alert rule, registry, user', async () => {
    const bin = getBinaryPath();

    // -----------------------------------------------------------------------
    // Phase 1: start fresh server, seed data
    // -----------------------------------------------------------------------
    srv1 = await startServer(bin);
    const { baseURL, dataDir, appsDir } = srv1;

    await setupAdmin(baseURL);
    const cookie = await login(baseURL);
    const api = at(baseURL, cookie);

    // Deploy nginx app
    const deployRes = await api('POST', '/api/apps/deploy', {
      name: 'dr-nginx',
      compose: COMPOSE_NGINX,
    });
    expect(deployRes.ok, `deploy failed: ${JSON.stringify(deployRes.data)}`).toBe(true);

    // Wait for app to appear in list (reconciler may take a moment)
    let appFound = false;
    const appDeadline = Date.now() + 30_000;
    while (Date.now() < appDeadline) {
      const r = await api('GET', '/api/apps', null);
      if (r.ok && Array.isArray(r.data) && r.data.some((a) => a.slug === 'dr-nginx' || a.Slug === 'dr-nginx')) {
        appFound = true;
        break;
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(appFound, 'dr-nginx not visible in /api/apps after deploy').toBe(true);

    // Create webhook
    const whRes = await api('POST', '/api/webhooks', {
      name: 'dr-slack',
      url: 'https://hooks.slack.com/fake/dr-test',
      type: 'slack',
      events: ['deploy_success'],
    });
    expect(whRes.ok, `webhook create failed: ${JSON.stringify(whRes.data)}`).toBe(true);
    const webhookId = whRes.data.id || whRes.data.ID;
    expect(webhookId).toBeTruthy();

    // Create alert rule
    const ruleRes = await api('POST', '/api/alerts/rules', {
      app_slug: 'dr-nginx',
      webhook_id: webhookId,
      metric: 'cpu',
      condition: '>',
      threshold: 90,
      duration: '5m',
      enabled: true,
    });
    expect(ruleRes.ok, `alert rule create failed: ${JSON.stringify(ruleRes.data)}`).toBe(true);
    const ruleId = ruleRes.data.id || ruleRes.data.ID;
    expect(ruleId).toBeTruthy();

    // Create registry
    const regRes = await api('POST', '/api/registries', {
      name: 'dr-ghcr',
      url: 'ghcr.io',
      username: 'dr-user',
      password: 'dr-fake-token',
    });
    expect(regRes.ok, `registry create failed: ${JSON.stringify(regRes.data)}`).toBe(true);
    const registryId = regRes.data.id || regRes.data.ID;
    expect(registryId).toBeTruthy();

    // Create additional user
    const userRes = await api('POST', '/api/users', {
      username: 'dr-editor',
      password: 'DrEditor123!',
      role: 'editor',
    });
    expect(userRes.ok, `user create failed: ${JSON.stringify(userRes.data)}`).toBe(true);

    // -----------------------------------------------------------------------
    // Phase 2: verify sidecars exist
    // -----------------------------------------------------------------------
    // Give syncer a moment to flush sidecars
    await new Promise((r) => setTimeout(r, 1_000));

    const globalSidecar = join(dataDir, 'config.yml');
    const appSidecar = join(appsDir, 'dr-nginx', 'simpledeploy.yml');

    expect(existsSync(globalSidecar), `global sidecar missing: ${globalSidecar}`).toBe(true);
    expect(existsSync(appSidecar), `app sidecar missing: ${appSidecar}`).toBe(true);

    // -----------------------------------------------------------------------
    // Phase 3: stop server 1, wipe DB
    // -----------------------------------------------------------------------
    stopProc(srv1);
    await new Promise((r) => setTimeout(r, 500));

    wipeDB(dataDir);

    expect(existsSync(join(dataDir, 'simpledeploy.db')), 'DB should be deleted').toBe(false);
    // Sidecars must still be present
    expect(existsSync(globalSidecar), 'global sidecar must survive wipe').toBe(true);
    expect(existsSync(appSidecar), 'app sidecar must survive wipe').toBe(true);

    // -----------------------------------------------------------------------
    // Phase 4: start second server with SAME dataDir + appsDir
    // -----------------------------------------------------------------------
    const { port: port2, proxyPort: proxyPort2 } = srv1; // ports are freed; grab new ones via startServer
    const srv2 = await startServer(bin, { dataDir, appsDir });
    // Update srv1 ref so afterAll cleanup handles dirs
    srv1 = null; // dirs already owned by srv2; prevent double-delete in afterAll

    const baseURL2 = srv2.baseURL;

    try {
      // -----------------------------------------------------------------------
      // Phase 5: login with same admin creds (proves user recovery from config.yml)
      // -----------------------------------------------------------------------
      const cookie2 = await login(baseURL2);
      const api2 = at(baseURL2, cookie2);

      // -----------------------------------------------------------------------
      // Phase 6: assert config recovery
      // -----------------------------------------------------------------------

      // App recovered
      const appsRes = await api2('GET', '/api/apps', null);
      expect(appsRes.ok, `GET /api/apps failed: ${appsRes.status}`).toBe(true);
      const apps = Array.isArray(appsRes.data) ? appsRes.data : [];
      const recoveredApp = apps.find((a) => (a.slug || a.Slug) === 'dr-nginx');
      expect(recoveredApp, 'dr-nginx app not recovered after DB wipe').toBeTruthy();

      // Webhook recovered
      const whListRes = await api2('GET', '/api/webhooks', null);
      expect(whListRes.ok, `GET /api/webhooks failed: ${whListRes.status}`).toBe(true);
      const webhooks = Array.isArray(whListRes.data) ? whListRes.data : [];
      const recoveredWh = webhooks.find((w) => (w.name || w.Name) === 'dr-slack');
      expect(recoveredWh, 'dr-slack webhook not recovered').toBeTruthy();

      // Alert rule recovered
      const rulesRes = await api2('GET', '/api/alerts/rules', null);
      expect(rulesRes.ok, `GET /api/alerts/rules failed: ${rulesRes.status}`).toBe(true);
      const rules = Array.isArray(rulesRes.data) ? rulesRes.data : [];
      const recoveredRule = rules.find(
        (r) => (r.app_slug || r.AppSlug) === 'dr-nginx' && (r.metric || r.Metric) === 'cpu',
      );
      expect(recoveredRule, 'alert rule not recovered').toBeTruthy();

      // Registry recovered
      const regListRes = await api2('GET', '/api/registries', null);
      expect(regListRes.ok, `GET /api/registries failed: ${regListRes.status}`).toBe(true);
      const registries = Array.isArray(regListRes.data) ? regListRes.data : [];
      const recoveredReg = registries.find((r) => (r.name || r.Name) === 'dr-ghcr');
      expect(recoveredReg, 'dr-ghcr registry not recovered').toBeTruthy();

      // Extra user recovered
      const usersRes = await api2('GET', '/api/users', null);
      expect(usersRes.ok, `GET /api/users failed: ${usersRes.status}`).toBe(true);
      const users = Array.isArray(usersRes.data) ? usersRes.data : [];
      const recoveredUser = users.find((u) => (u.username || u.Username) === 'dr-editor');
      expect(recoveredUser, 'dr-editor user not recovered').toBeTruthy();

      // -----------------------------------------------------------------------
      // Phase 7: historical tables are empty (no metrics, no deploy events)
      // -----------------------------------------------------------------------
      const metricsRes = await api2('GET', '/api/apps/dr-nginx/metrics?range=1h', null);
      // Endpoint may return 200 with empty data or 404 if no metrics recorded.
      // Either is acceptable; what matters is no crash.
      if (metricsRes.ok && metricsRes.data && typeof metricsRes.data === 'object') {
        const series = metricsRes.data.series || metricsRes.data.data || [];
        // series may be an empty array or contain zero data points; both are fine.
        // We just assert it's an array (no unexpected shape).
        expect(Array.isArray(series) || series === null).toBe(true);
      }

      const eventsRes = await api2('GET', '/api/apps/dr-nginx/events', null);
      if (eventsRes.ok) {
        const events = Array.isArray(eventsRes.data) ? eventsRes.data : [];
        // Deploy events are historical; should be empty after DB wipe.
        expect(events.length).toBe(0);
      }
    } finally {
      stopProc(srv2);
      for (const dir of [srv2.dataDir, srv2.appsDir]) {
        try { rmSync(dir, { recursive: true, force: true }); } catch {}
      }
    }
  });
});
