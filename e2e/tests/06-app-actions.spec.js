import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import { apiRequest, apiLogin, waitForAppStatus } from '../helpers/api.js';
import {
  findServiceContainer,
  containerRunning,
  dockerInspect,
  waitForContainerState,
  countServiceReplicas,
} from '../helpers/docker.js';
import { fetchViaProxy } from '../helpers/proxy.js';

test.describe('App Actions', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('stop app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /stop/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const confirmBtn = dialog.getByRole('button', { name: /stop|confirm|yes/i });
      if (await confirmBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await confirmBtn.click();
      }
    }
    await expect(page.getByText(/stopped/i).first()).toBeVisible({ timeout: 30_000 });
  });

  test('start app', async ({ page }) => {
    const state = getState();
    // Ensure stopped via API to avoid cross-test ordering flakiness.
    await apiLogin('e2eadmin', 'E2eTestPass123!');
    await apiRequest('POST', '/api/apps/e2e-nginx/stop');
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx');
      if (res.ok && res.data?.status === 'stopped') break;
      await new Promise(r => setTimeout(r, 1_000));
    }
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await expect(page.getByRole('button', { name: 'Start' })).toBeVisible({ timeout: 30_000 });
    await page.getByRole('button', { name: 'Start' }).click();
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 30_000 });
  });

  test('restart app', async ({ page }) => {
    const state = getState();
    // Use the API to ensure app is running before testing restart
    const { apiRequest, apiLogin } = await import('../helpers/api.js');
    await apiLogin('e2eadmin', 'E2eTestPass123!');
    await apiRequest('POST', '/api/apps/e2e-nginx/start');
    // Wait for Docker to start the container
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx');
      if (res.ok && res.data?.status === 'running') break;
      await new Promise(r => setTimeout(r, 2_000));
    }
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await expect(page.getByRole('button', { name: /restart/i })).toBeVisible({ timeout: 15_000 });
    await page.getByRole('button', { name: /restart/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const confirmBtn = dialog.getByRole('button', { name: /restart|confirm/i });
      if (await confirmBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await confirmBtn.click();
      }
    }
    // Wait for action modal close button (inside the dialog, not the backdrop)
    const actionDialog = page.getByRole('dialog');
    const closeBtn = actionDialog.locator('button:has-text("Close"):not([aria-label])');
    await expect(closeBtn).toBeVisible({ timeout: 60_000 });
    await closeBtn.click();
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 15_000 });
  });

  test('pull and update', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /pull/i }).click();
    // Wait for action modal close button (inside dialog footer, not backdrop)
    const actionDialog = page.getByRole('dialog');
    const closeBtn = actionDialog.locator('button:has-text("Close"):not([aria-label])');
    await expect(closeBtn).toBeVisible({ timeout: 120_000 });
    await closeBtn.click();
  });

  test('scale service', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-multi`);
    // Scale is inside the "..." (more) dropdown menu
    const moreBtn = page.locator('button').filter({ has: page.locator('svg path[d*="6.75 12a.75"]') });
    await moreBtn.click();
    await page.locator('button').filter({ hasText: 'Scale' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    const inputs = dialog.locator('input[type="number"]');
    const count = await inputs.count();
    if (count > 0) {
      await inputs.first().fill('2');
    }
    const applyBtn = dialog.getByRole('button', { name: /apply/i });
    await applyBtn.click();
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 30_000 });
  });
});

test.describe.configure({ mode: 'serial' });
test.describe('App Actions - Functional', () => {
  test.beforeAll(async () => {
    await apiLogin('e2eadmin', 'E2eTestPass123!');
  });

  test('stop actually stops container and proxy returns upstream error', async () => {
    // Ensure running first.
    await apiRequest('POST', '/api/apps/e2e-nginx/start');
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name).toBeTruthy();
    await waitForContainerState(name, true, 30_000);

    const res = await apiRequest('POST', '/api/apps/e2e-nginx/stop');
    expect(res.ok).toBe(true);
    await waitForContainerState(name, false, 30_000);
    expect(containerRunning(name)).toBe(false);

    // With container down, proxy should fail to reach upstream.
    const r = await fetchViaProxy('nginx-test.local', '/');
    expect([502, 503, 504, 500, 0, 404].includes(r.status)).toBe(true);
    expect(r.status).not.toBe(200);
  });

  test('start actually starts container and proxy serves 200', async () => {
    const res = await apiRequest('POST', '/api/apps/e2e-nginx/start');
    expect(res.ok).toBe(true);
    const name = findServiceContainer('e2e-nginx', 'web');
    await waitForContainerState(name, true, 30_000);
    expect(containerRunning(name)).toBe(true);

    // Give proxy a moment to re-route.
    const deadline = Date.now() + 30_000;
    let last = 0;
    while (Date.now() < deadline) {
      const r = await fetchViaProxy('nginx-test.local', '/');
      last = r.status;
      if (r.status === 200) break;
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(last).toBe(200);
  });

  test('restart preserves container but bumps StartedAt', async () => {
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name).toBeTruthy();
    const beforeStart = dockerInspect(name).State.StartedAt;
    const beforeMs = new Date(beforeStart).getTime();

    const res = await apiRequest('POST', '/api/apps/e2e-nginx/restart');
    expect(res.ok).toBe(true);

    const deadline = Date.now() + 60_000;
    let advanced = false;
    while (Date.now() < deadline) {
      try {
        const after = dockerInspect(findServiceContainer('e2e-nginx', 'web')).State.StartedAt;
        const afterMs = new Date(after).getTime();
        if (afterMs > beforeMs) {
          advanced = true;
          break;
        }
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(advanced).toBe(true);
  });

  test('scale to 2 creates 2nd replica then back to 1', async () => {
    // Use the cache (redis) service since web has a fixed host-port mapping
    // that would conflict when scaled to multiple replicas.
    test.setTimeout(180_000);
    const up = await apiRequest('POST', '/api/apps/e2e-multi/scale', { scales: { cache: 2 } });
    expect(up.ok, `scale up failed: ${JSON.stringify(up.data)}`).toBe(true);

    const deadline = Date.now() + 60_000;
    while (Date.now() < deadline) {
      if (countServiceReplicas('e2e-multi', 'cache') === 2) break;
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(countServiceReplicas('e2e-multi', 'cache')).toBe(2);

    const down = await apiRequest('POST', '/api/apps/e2e-multi/scale', { scales: { cache: 1 } });
    expect(down.ok).toBe(true);

    const deadline2 = Date.now() + 60_000;
    while (Date.now() < deadline2) {
      if (countServiceReplicas('e2e-multi', 'cache') === 1) break;
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(countServiceReplicas('e2e-multi', 'cache')).toBe(1);
  });

  test('pull keeps app running and serving traffic', async () => {
    test.setTimeout(180_000);
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name).toBeTruthy();
    const beforeImage = dockerInspect(name).Image;

    const res = await apiRequest('POST', '/api/apps/e2e-nginx/pull');
    expect([200, 202].includes(res.status)).toBe(true);

    // pull is async; give it time to complete.
    await new Promise((r) => setTimeout(r, 20_000));
    // Wait for running status.
    await waitForAppStatus('e2e-nginx', 'running', 60_000);

    const currentName = findServiceContainer('e2e-nginx', 'web');
    expect(currentName).toBeTruthy();
    expect(containerRunning(currentName)).toBe(true);
    // Image digest exists (may or may not change depending on registry state).
    const afterImage = dockerInspect(currentName).Image;
    expect(afterImage).toBeTruthy();
    expect(typeof beforeImage).toBe('string');

    const r = await fetchViaProxy('nginx-test.local', '/');
    expect(r.status).toBe(200);
  });
});
