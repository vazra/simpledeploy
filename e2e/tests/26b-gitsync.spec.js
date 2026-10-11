/**
 * Git-sync roundtrip + conflict spec.
 *
 * Requires: git CLI, Docker daemon, built binary (make build-go or make build).
 * Gate: only runs when E2E_GITSYNC=1.
 *
 * Spawns its own isolated server (fresh data + apps dirs). Does NOT touch the
 * shared server from global-setup.
 *
 * Run standalone:
 *   cd e2e
 *   E2E_GITSYNC=1 npx playwright test 26b-gitsync.spec.js --reporter=list
 */

import { test, expect } from '@playwright/test';
import { execFileSync, execSync, spawn } from 'child_process';
import { mkdtempSync, writeFileSync, rmSync, existsSync, readFileSync, createWriteStream, renameSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import { createHmac } from 'crypto';
import net from 'net';
import { getBinaryPath } from '../helpers/server.js';
import { apiRequestAt } from '../helpers/api.js';

// ---------------------------------------------------------------------------
// Gate: skip entire file when E2E_GITSYNC is not set
// ---------------------------------------------------------------------------

const ENABLED = !!process.env.E2E_GITSYNC;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const ADMIN = {
  username: 'gs-admin',
  password: 'GsTestPass123!',
  displayName: 'GS Admin',
  email: 'gs@test.local',
};

const WEBHOOK_SECRET = 'test-secret';
const APP_SLUG = 'gs-nginx';

const COMPOSE_NGINX = `services:
  web:
    image: nginx:alpine
`;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getAvailablePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, () => {
      const port = srv.address().port;
      srv.close(() => resolve(port));
    });
    srv.on('error', reject);
  });
}

async function setupAdmin(baseURL) {
  const res = await apiRequestAt(baseURL, 'POST', '/api/setup', ADMIN, null);
  if (!res.ok) throw new Error(`setup failed: ${res.status} ${JSON.stringify(res.data)}`);
  return res;
}

async function login(baseURL) {
  const res = await apiRequestAt(baseURL, 'POST', '/api/auth/login', {
    username: ADMIN.username,
    password: ADMIN.password,
  }, null);
  if (!res.ok) throw new Error(`login failed: ${res.status} ${JSON.stringify(res.data)}`);
  if (!res.setCookie) throw new Error('no session cookie after login');
  return res.setCookie;
}

function at(baseURL, cookie) {
  return (method, path, body) => apiRequestAt(baseURL, method, path, body, cookie);
}

function hubSig(secret, rawBody) {
  return 'sha256=' + createHmac('sha256', secret).update(rawBody).digest('hex');
}

async function triggerWebhook(baseURL, cookie, secret, body) {
  const rawBody = typeof body === 'string' ? body : JSON.stringify(body);
  const sig = hubSig(secret, rawBody);
  const res = await fetch(`${baseURL}/api/git/webhook`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Hub-Signature-256': sig,
      ...(cookie ? { Cookie: cookie } : {}),
    },
    body: rawBody,
  });
  const text = await res.text();
  return { status: res.status, ok: res.ok, text };
}

async function poll(fn, timeoutMs = 10_000, intervalMs = 500) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    try {
      const result = await fn();
      if (result) return result;
      last = result;
    } catch (e) {
      last = e;
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  throw new Error(`poll timed out after ${timeoutMs}ms; last=${JSON.stringify(last)}`);
}

// Contents of a file on the bare remote's branch, or null if absent.
function remoteFile(bareRepoPath, path, branch = 'main') {
  try {
    return execFileSync('git', ['--git-dir', bareRepoPath, 'show', `${branch}:${path}`], {
      encoding: 'utf-8',
      stdio: ['ignore', 'pipe', 'ignore'],
    });
  } catch {
    return null;
  }
}

function stopProc(srv) {
  if (!srv) return;
  try { srv.proc.kill('SIGTERM'); } catch {}
}

const ROOT = join(import.meta.dirname, '..', '..');

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

test.describe('Git sync: roundtrip + conflict', () => {
  test.describe.configure({ mode: 'serial' });
  test.setTimeout(120_000);

  // Skip entire suite if gate not set
  test.skip(!ENABLED, 'Set E2E_GITSYNC=1 to run git-sync tests');

  let srv = null;
  let baseURL = null;
  let cookie = null;
  let api = null;
  let bareRepoPath = null;
  let cloneDir = null;
  let webhookId = null;

  test.beforeAll(async () => {
    const bin = getBinaryPath();

    // 1. Create an empty bare repo: SimpleDeploy refuses to push into a
    // remote that already has commits, and pushes the initial commit itself.
    bareRepoPath = join(tmpdir(), `sd-gitsync-bare-${Date.now()}.git`);
    execFileSync('git', ['init', '--bare', '-b', 'main', bareRepoPath]);

    // 2. Allocate ports, write config with gitsync block
    const mgmtPort = await getAvailablePort();
    const proxyPort = await getAvailablePort();
    const dataDir = mkdtempSync(join(tmpdir(), 'sd-e2e-gs-data-'));
    const appsDir = mkdtempSync(join(tmpdir(), 'sd-e2e-gs-apps-'));
    // Not data_dir/config.yml: that path is SimpleDeploy's own global sidecar.
    const configPath = join(dataDir, 'server-config.yaml');
    const logPath = join(dataDir, 'server.log');

    const configContent = [
      `data_dir: "${dataDir}"`,
      `apps_dir: "${appsDir}"`,
      `listen_addr: ":${proxyPort}"`,
      `management_port: ${mgmtPort}`,
      `master_secret: "e2e-test-secret-key-32bytes!!"`,
      `log_buffer_size: 100`,
      `tls:`,
      `  mode: "off"`,
      `ratelimit:`,
      `  requests: 10000`,
      `  window: "60s"`,
      `  burst: 5000`,
      `  by: "ip"`,
      `git_sync:`,
      `  enabled: true`,
      `  remote: "file://${bareRepoPath}"`,
      `  branch: main`,
      `  poll_interval: 1s`,
      `  webhook_secret: "${WEBHOOK_SECRET}"`,
    ].join('\n');
    writeFileSync(configPath, configContent);

    // 3. Start server
    const logStream = createWriteStream(logPath);
    const proc = spawn(bin, ['serve', '--config', configPath], {
      cwd: ROOT,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS: '1' },
    });
    proc.stdout.pipe(logStream);
    proc.stderr.pipe(logStream);
    proc.on('error', (err) => console.error('[e2e:gitsync] server error:', err));

    baseURL = `http://localhost:${mgmtPort}`;
    srv = { proc, dataDir, appsDir, configPath, logPath, baseURL, port: mgmtPort, proxyPort };

    // Wait for healthy
    const deadline = Date.now() + 30_000;
    let healthy = false;
    while (Date.now() < deadline) {
      try {
        const r = await fetch(`${baseURL}/api/health`);
        if (r.ok) { healthy = true; break; }
      } catch {}
      await new Promise((r) => setTimeout(r, 300));
    }
    if (!healthy) {
      proc.kill('SIGKILL');
      const logs = existsSync(logPath) ? readFileSync(logPath, 'utf-8') : '(no logs)';
      throw new Error(`Server failed to start within 30s.\nLogs:\n${logs}`);
    }

    // 4. Admin setup + login
    await setupAdmin(baseURL);
    cookie = await login(baseURL);
    api = at(baseURL, cookie);
  });

  test.afterAll(async () => {
    stopProc(srv);
    await new Promise((r) => setTimeout(r, 500));
    for (const dir of [srv?.dataDir, srv?.appsDir, cloneDir, bareRepoPath]) {
      if (dir) try { rmSync(dir, { recursive: true, force: true }); } catch {}
    }
  });

  // -------------------------------------------------------------------------
  // Test 0: Test-connection endpoint happy path
  // -------------------------------------------------------------------------

  test('Test-connection endpoint reports OK against the bare repo', async () => {
    const res = await api('POST', '/api/git/test-connection', {
      remote: `file://${bareRepoPath}`,
      branch: 'main',
    });
    expect(res.ok, `test-connection failed: ${JSON.stringify(res.data)}`).toBe(true);
    expect(res.data.ok).toBe(true);
    expect(res.data.code).toBe('ok');
    expect(res.data.branch_found).toBe(true);
    expect(res.data.message).toMatch(/Connected/i);

    // Branch missing case
    const missing = await api('POST', '/api/git/test-connection', {
      remote: `file://${bareRepoPath}`,
      branch: 'does-not-exist',
    });
    expect(missing.ok).toBe(true);
    expect(missing.data.ok).toBe(false);
    expect(missing.data.code).toBe('branch_missing');
  });

  // -------------------------------------------------------------------------
  // Test 1: UI changes commit to remote
  // -------------------------------------------------------------------------

  test('UI change commits to remote', async () => {
    // Deploy nginx app via API
    const deployRes = await api('POST', '/api/apps/deploy', {
      name: APP_SLUG,
      compose: Buffer.from(COMPOSE_NGINX).toString('base64'),
    });
    expect(deployRes.ok, `deploy failed: ${JSON.stringify(deployRes.data)}`).toBe(true);

    // Wait for app to appear in list
    await poll(async () => {
      const r = await api('GET', '/api/apps', null);
      return r.ok && Array.isArray(r.data) && r.data.some((a) => (a.slug || a.Slug) === APP_SLUG);
    }, 30_000);

    // Create a webhook
    const whRes = await api('POST', '/api/webhooks', {
      name: 'gs-slack',
      url: 'https://hooks.slack.com/fake/gs-test',
      type: 'slack',
      events: ['deploy_success'],
    });
    expect(whRes.ok, `webhook create failed: ${JSON.stringify(whRes.data)}`).toBe(true);
    webhookId = whRes.data.id || whRes.data.ID;
    expect(webhookId).toBeTruthy();

    // Wait until the server has pushed the app and the webhook.
    await poll(
      () => remoteFile(bareRepoPath, `${APP_SLUG}/docker-compose.yml`) && remoteFile(bareRepoPath, '_global.yml')?.includes('gs-slack'),
      20_000,
    );

    // Clone bare repo and assert files
    cloneDir = mkdtempSync(join(tmpdir(), 'sd-gitsync-clone-'));
    execFileSync('git', ['clone', `file://${bareRepoPath}`, cloneDir]);

    const composePath = join(cloneDir, APP_SLUG, 'docker-compose.yml');
    const globalPath = join(cloneDir, '_global.yml');

    expect(existsSync(composePath), `${APP_SLUG}/docker-compose.yml missing from remote`).toBe(true);
    // The per-app sidecar is written once the app has settings (test 2 adds an alert rule).
    expect(existsSync(globalPath), '_global.yml missing from remote').toBe(true);

    const globalContent = readFileSync(globalPath, 'utf-8');
    expect(globalContent).toContain('gs-slack');
    // URL must NOT be present in _global.yml (redacted)
    expect(globalContent).not.toContain('hooks.slack.com');

    // Last commit message should identify simpledeploy-sync
    const logOut = execFileSync('git', ['log', '-1', '--format=%B'], { cwd: cloneDir }).toString();
    expect(logOut).toContain('simpledeploy-sync');
  });

  // -------------------------------------------------------------------------
  // Test 2: Remote push reconciles into server
  // -------------------------------------------------------------------------

  test('Remote push reconciles into server', async () => {
    expect(cloneDir, 'cloneDir must be set from previous test').toBeTruthy();
    expect(webhookId, 'webhookId must be set from previous test').toBeTruthy();

    // Create alert rule so simpledeploy.yml has alert_rules
    const appsRes = await api('GET', '/api/apps', null);
    const app = (appsRes.data || []).find((a) => (a.Slug || a.slug) === APP_SLUG);
    expect(app, `app ${APP_SLUG} not found`).toBeTruthy();
    const ruleRes = await api('POST', '/api/alerts/rules', {
      app_id: app.ID ?? app.id,
      webhook_id: webhookId,
      metric: 'cpu_pct',
      operator: '>',
      threshold: 80,
      duration_sec: 300,
      enabled: true,
    });
    expect(ruleRes.ok, `alert rule create failed: ${JSON.stringify(ruleRes.data)}`).toBe(true);

    // Wait for sidecar to be committed and pushed
    await new Promise((r) => setTimeout(r, 6_000));

    // Pull latest into clone
    execFileSync('git', ['pull'], { cwd: cloneDir });

    const sidecarPath = join(cloneDir, APP_SLUG, 'simpledeploy.yml');
    expect(existsSync(sidecarPath), 'simpledeploy.yml not in clone after pull').toBe(true);

    // Modify alert threshold
    let sidecarContent = readFileSync(sidecarPath, 'utf-8');
    expect(sidecarContent, 'threshold 80 not found in sidecar').toContain('80');
    sidecarContent = sidecarContent.replace(/threshold:\s*80/, 'threshold: 95');
    writeFileSync(sidecarPath, sidecarContent);

    // Commit and push
    const gitEnv = {
      ...process.env,
      GIT_AUTHOR_NAME: 'test',
      GIT_AUTHOR_EMAIL: 'test@test',
      GIT_COMMITTER_NAME: 'test',
      GIT_COMMITTER_EMAIL: 'test@test',
    };
    execFileSync('git', ['add', sidecarPath], { cwd: cloneDir, env: gitEnv });
    execFileSync('git', ['commit', '-m', 'chore: bump cpu threshold to 95'], { cwd: cloneDir, env: gitEnv });
    execFileSync('git', ['push'], { cwd: cloneDir, env: gitEnv });

    // Trigger webhook to force immediate pull
    const body = JSON.stringify({ ref: 'refs/heads/main' });
    const wh = await triggerWebhook(baseURL, cookie, WEBHOOK_SECRET, body);
    expect([200, 202, 204], `webhook returned ${wh.status}: ${wh.text}`).toContain(wh.status);

    // Assert alert rule threshold updated to 95
    const rule = await poll(async () => {
      const r = await api('GET', '/api/alerts/rules', null);
      if (!r.ok || !Array.isArray(r.data)) return null;
      const found = r.data.find(
        (x) => (x.app_slug || x.AppSlug) === APP_SLUG && (x.metric || x.Metric) === 'cpu_pct',
      );
      if (!found) return null;
      const thresh = found.threshold ?? found.Threshold;
      return thresh === 95 ? found : null;
    }, 20_000);

    expect(rule, 'threshold not updated to 95 after remote push + webhook trigger').toBeTruthy();
  });

  // -------------------------------------------------------------------------
  // Test 3: Conflict surfaces in status, local wins
  // -------------------------------------------------------------------------

  // A real conflict needs both sides to change the same file before the
  // server sees the remote change. With a 1s poll that only happens while
  // the remote is unreachable, so take it offline, edit on the server (the
  // commit stays local), restore it, and push a conflicting edit.
  test('Conflict while remote unreachable: recorded in status, server version wins', async () => {
    expect(cloneDir, 'cloneDir must be set from previous tests').toBeTruthy();
    expect(webhookId, 'webhookId must be set from test 1').toBeTruthy();

    const gitEnv = {
      ...process.env,
      GIT_AUTHOR_NAME: 'test',
      GIT_AUTHOR_EMAIL: 'test@test',
      GIT_COMMITTER_NAME: 'test',
      GIT_COMMITTER_EMAIL: 'test@test',
    };
    // Wait until the server has pushed everything, then let the post-pull
    // window pass (suppressTail, 1.2s): writes inside it count as re-renders
    // of pulled files, not dashboard edits.
    const git = (cwd, ...args) => execFileSync('git', args, { cwd, env: gitEnv, encoding: 'utf-8' }).trim();
    await poll(() => git(srv.appsDir, 'rev-parse', 'HEAD') === git(bareRepoPath, 'rev-parse', 'main'), 20_000);
    await new Promise((r) => setTimeout(r, 2_000));

    const offlinePath = `${bareRepoPath}.offline`;
    renameSync(bareRepoPath, offlinePath);
    try {
      const updateRes = await api('PUT', `/api/webhooks/${webhookId}`, {
        name: 'from-server',
        url: 'https://hooks.slack.com/fake/gs-test',
        type: 'slack',
        events: ['deploy_success'],
      });
      expect(updateRes.ok, `webhook update failed: ${JSON.stringify(updateRes.data)}`).toBe(true);

      // The server commits locally; the push cannot reach the remote.
      await poll(() => {
        try {
          return execFileSync('git', ['show', 'HEAD:_global.yml'], { cwd: srv.appsDir, encoding: 'utf-8' }).includes('from-server');
        } catch {
          return false;
        }
      }, 15_000);

      // Conflicting edit to the same line, pushed straight into the offline
      // repo so the server cannot see it before its own commit exists.
      git(cloneDir, 'pull', '-q', `file://${offlinePath}`, 'main');
      const globalPath = join(cloneDir, '_global.yml');
      const before = readFileSync(globalPath, 'utf-8');
      const renamed = before.replace(/(webhooks:\s*\n\s*- name: )\S+/, '$1from-remote');
      expect(renamed, 'webhook entry not found in _global.yml').not.toBe(before);
      writeFileSync(globalPath, renamed);
      git(cloneDir, 'commit', '-qam', 'remote: rename webhook to from-remote');
      git(cloneDir, 'push', '-q', `file://${offlinePath}`, 'HEAD:main');
    } finally {
      renameSync(offlinePath, bareRepoPath);
    }

    const status = await poll(async () => {
      const r = await api('GET', '/api/git/status', null);
      return r.ok && (r.data.RecentConflicts || []).some((c) => c.Path === '_global.yml') ? r.data : null;
    }, 20_000);
    expect(status.RecentConflicts.find((c) => c.Path === '_global.yml').RemoteSHA).toBeTruthy();

    // Server version wins in the dashboard and is pushed back to the remote.
    const whListRes = await api('GET', '/api/webhooks', null);
    expect(whListRes.ok).toBe(true);
    const serverWh = whListRes.data.find((w) => w.id === webhookId);
    expect(serverWh?.name, 'dashboard webhook name should stay from-server').toBe('from-server');
    await poll(() => remoteFile(bareRepoPath, '_global.yml')?.includes('from-server'), 20_000);
    expect(remoteFile(bareRepoPath, '_global.yml')).not.toContain('from-remote');
  });
});
