// 26a-config-recovery.spec.js
//
// Disaster recovery from config sidecars (docs/operations/config-sidecars.md):
// stop the server, delete the SQLite DB, start again with the same data and
// apps dirs. Users, API keys, registries and webhooks come back from
// {data_dir}/config.yml (+ secrets.yml); alert rules and access grants from
// {apps_dir}/<app>/simpledeploy.yml. Runs on its own isolated server.

import { test, expect } from '@playwright/test';
import { existsSync, readFileSync, rmSync } from 'fs';
import { join } from 'path';
import {
  startIsolatedServer, stopIsolatedServer, stopGracefully, isRunning, loginAt, deployAt, waitForAppStatusAt,
} from '../helpers/isolated.js';

const ADMIN = { username: 'dr-admin', password: 'DrTestPass123!', displayName: 'DR Admin', email: 'dr@test.local' };
const EDITOR = { username: 'dr-editor', password: 'DrEditor123!' };
const APP = 'dr-nginx';
const COMPOSE = 'services:\n  web:\n    image: nginx:alpine\n';

test.describe('DR: sidecar recovery after DB wipe', () => {
  let server = null;
  let api = null;

  test.afterAll(async () => {
    if (!server) return;
    if (isRunning(server)) await api('DELETE', `/api/apps/${APP}`);
    await stopIsolatedServer(server, '26a-config-recovery');
  });

  test('restart on a wiped DB restores config from the sidecars', async () => {
    test.setTimeout(240_000);
    let cookie;
    ({ server, cookie, api } = await startIsolatedServer({ admin: ADMIN }));
    const { dataDir, appsDir } = server;

    // Seed one of each mirrored setting.
    await deployAt(api, APP, COMPOSE);
    const app = await waitForAppStatusAt(server.baseURL, cookie, APP, 'running');

    const wh = await api('POST', '/api/webhooks', { name: 'dr-hook', type: 'slack', url: 'http://127.0.0.1:9/dr-hook' });
    expect(wh.status, JSON.stringify(wh.data)).toBe(201);
    const rule = await api('POST', '/api/alerts/rules', {
      app_id: app.ID, webhook_id: wh.data.id, metric: 'cpu_pct', operator: '>', threshold: 90, duration_sec: 300, enabled: true,
    });
    expect(rule.status, JSON.stringify(rule.data)).toBe(201);
    const reg = await api('POST', '/api/registries', { name: 'dr-ghcr', url: 'ghcr.io', username: 'dr-user', password: 'dr-fake-token' });
    expect(reg.status, JSON.stringify(reg.data)).toBe(201);
    const user = await api('POST', '/api/users', { ...EDITOR, role: 'manage', display_name: 'DR Editor', email: 'editor@dr.local' });
    expect(user.status, JSON.stringify(user.data)).toBe(201);
    const grant = await api('POST', `/api/users/${user.data.id}/access`, { app_slug: APP });
    expect(grant.status, JSON.stringify(grant.data)).toBe(200);
    const key = await api('POST', '/api/apikeys', { name: 'dr-key' });
    expect(key.status, JSON.stringify(key.data)).toBe(201);

    // Stop right after the last change: shutdown must flush the debounced
    // sidecar writes.
    await stopGracefully(server);

    const globalSidecar = readFileSync(join(dataDir, 'config.yml'), 'utf-8');
    for (const re of [/username: dr-admin/, /username: dr-editor/, /name: dr-hook/, /name: dr-ghcr/, /name: dr-key/]) {
      expect(globalSidecar, 'config.yml after shutdown').toMatch(re);
    }
    const appSidecarPath = join(appsDir, APP, 'simpledeploy.yml');
    const appSidecar = readFileSync(appSidecarPath, 'utf-8');
    for (const re of [/metric: cpu_pct/, /webhook: dr-hook/, /username: dr-editor/]) {
      expect(appSidecar, 'simpledeploy.yml after shutdown').toMatch(re);
    }

    for (const f of ['simpledeploy.db', 'simpledeploy.db-wal', 'simpledeploy.db-shm']) {
      rmSync(join(dataDir, f), { force: true });
    }
    expect(existsSync(join(dataDir, 'simpledeploy.db'))).toBe(false);

    // Same dirs, fresh ports. Logging in with the old admin password proves
    // the users table came back from config.yml.
    ({ server, cookie, api } = await startIsolatedServer({ setup: false, admin: ADMIN, dataDir, appsDir, logName: '26a-restart' }));
    await expect.poll(() => readFileSync(server.logPath, 'utf-8'), { timeout: 5_000 })
      .toContain('[configsync] imported global sidecar into empty DB');

    const users = await api('GET', '/api/users');
    expect(users.status).toBe(200);
    const editor = users.data.find((u) => u.username === EDITOR.username);
    expect(editor, JSON.stringify(users.data)).toMatchObject({ role: 'manage', display_name: 'DR Editor', email: 'editor@dr.local' });
    await loginAt(server.baseURL, EDITOR.username, EDITOR.password);

    const byKey = await fetch(`${server.baseURL}/api/apps`, { headers: { Authorization: `Bearer ${key.data.key}` } });
    expect(byKey.status, 'API key restored').toBe(200);

    const webhooks = await api('GET', '/api/webhooks');
    const hook = webhooks.data.find((w) => w.name === 'dr-hook');
    expect(hook, JSON.stringify(webhooks.data)).toMatchObject({ type: 'slack', url: 'http://127.0.0.1:9/dr-hook' });

    // Registry credentials are encrypted with master_secret; the decrypted
    // username proves they survived.
    const regs = await api('GET', '/api/registries');
    expect(regs.data.find((r) => r.name === 'dr-ghcr'), JSON.stringify(regs.data)).toMatchObject({ url: 'ghcr.io', username: 'dr-user' });

    // The reconciler re-adopts the app from apps_dir and imports its sidecar.
    await waitForAppStatusAt(server.baseURL, cookie, APP, 'running', 60_000);
    await expect.poll(async () => {
      const rules = await api('GET', '/api/alerts/rules');
      return rules.data.filter((r) => r.app_slug === APP)
        .map((r) => ({ metric: r.metric, operator: r.operator, threshold: r.threshold, duration_sec: r.duration_sec, webhook_id: r.webhook_id, enabled: r.enabled }));
    }, { message: 'alert rule restored from simpledeploy.yml', timeout: 20_000 })
      .toEqual([{ metric: 'cpu_pct', operator: '>', threshold: 90, duration_sec: 300, webhook_id: hook.id, enabled: true }]);
    const access = await api('GET', `/api/users/${editor.id}/access`);
    expect(access.data, 'access grant restored from simpledeploy.yml').toEqual([APP]);
  });
});
