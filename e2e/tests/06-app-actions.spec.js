import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiRequest, apiLogin, waitForAppStatus } from '../helpers/api.js';
import {
  findServiceContainer,
  dockerInspect,
  waitForContainerState,
  countServiceReplicas,
} from '../helpers/docker.js';
import { waitForProxyStatus } from '../helpers/proxy.js';

async function eventCount(slug, action) {
  const res = await apiRequest('GET', `/api/apps/${slug}/events`);
  expect(res.status).toBe(200);
  return res.data.filter((e) => e.action === action).length;
}

function actionResponse(page, slug, action) {
  return page.waitForResponse((r) =>
    r.url().endsWith(`/api/apps/${slug}/${action}`) && r.request().method() === 'POST');
}

// ActionModal streams the async action over WS and shows the done action.
async function expectActionComplete(page, action, timeout) {
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText('Complete', { exact: true })).toBeVisible({ timeout });
  await expect(dialog.getByText(action, { exact: true })).toBeVisible();
  await dialog.locator('button:has-text("Close"):not([aria-label])').click();
  await expect(dialog).toBeHidden();
}

// stop leaves e2e-nginx stopped for start; the rest expect it running.
test.describe('App Actions', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  // Later specs need e2e-nginx running even if start failed here.
  test.afterAll(async () => {
    const app = await apiRequest('GET', '/api/apps/e2e-nginx');
    if (app.data?.Status === 'stopped') await apiRequest('POST', '/api/apps/e2e-nginx/start');
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('stop stops the container and the proxy reports a bad gateway', async ({ page }) => {
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name).toBeTruthy();
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    const res = actionResponse(page, 'e2e-nginx', 'stop');
    await page.getByRole('button', { name: 'Stop', exact: true }).click();
    expect((await res).status()).toBe(200);
    await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeVisible();

    await waitForContainerState(name, false, 30_000);
    await waitForAppStatus('e2e-nginx', 'stopped', 5_000);
    await waitForProxyStatus('nginx-test.local', 502);
  });

  test('start brings the container back and the proxy serves it', async ({ page }) => {
    const name = findServiceContainer('e2e-nginx', 'web');
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    const res = actionResponse(page, 'e2e-nginx', 'start');
    await page.getByRole('button', { name: 'Start', exact: true }).click();
    expect((await res).status()).toBe(200);
    await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();

    await waitForContainerState(name, true, 30_000);
    await waitForAppStatus('e2e-nginx', 'running', 5_000);
    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 30_000, bodyContains: 'Welcome to nginx' });
  });

  test('restart restarts the container in place', async ({ page }) => {
    const name = findServiceContainer('e2e-nginx', 'web');
    const before = dockerInspect(name);
    const restarts = await eventCount('e2e-nginx', 'restart');

    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'Restart', exact: true }).click();
    const confirm = page.getByRole('dialog').filter({ hasText: 'Restart App' });
    const res = actionResponse(page, 'e2e-nginx', 'restart');
    await confirm.getByRole('button', { name: 'Restart', exact: true }).click();
    expect((await res).status()).toBe(202);
    await expect(confirm).toBeHidden();
    await expectActionComplete(page, 'restart', 60_000);

    const after = dockerInspect(name);
    expect(after.Id).toBe(before.Id);
    expect(new Date(after.State.StartedAt).getTime()).toBeGreaterThan(new Date(before.State.StartedAt).getTime());
    // The event row is written just after the WS done message.
    await expect.poll(() => eventCount('e2e-nginx', 'restart')).toBe(restarts + 1);
    await waitForAppStatus('e2e-nginx', 'running', 5_000);
    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 30_000, bodyContains: 'Welcome to nginx' });
  });

  test('pull & update re-pulls and keeps serving', async ({ page }) => {
    const pulls = await eventCount('e2e-nginx', 'pull');

    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    const res = actionResponse(page, 'e2e-nginx', 'pull');
    await page.getByRole('button', { name: 'Pull & Update' }).click();
    expect((await res).status()).toBe(202);
    await expectActionComplete(page, 'pull', 120_000);

    await expect.poll(() => eventCount('e2e-nginx', 'pull')).toBe(pulls + 1);
    await waitForAppStatus('e2e-nginx', 'running', 5_000);
    await waitForContainerState(findServiceContainer('e2e-nginx', 'web'), true, 5_000);
    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 30_000, bodyContains: 'Welcome to nginx' });
  });

  test('scale cache to 2 replicas and back to 1', async ({ page }) => {
    // web has a fixed host port, so only cache is scalable.
    await page.goto(`${getState().baseURL}/#/apps/e2e-multi`);
    const row = page.locator('main div.rounded-lg').filter({ has: page.locator('span[title="cache"]') });
    const replicas = row.locator('span[title="Current replicas"]');
    await expect(replicas).toHaveText('1');

    let res = actionResponse(page, 'e2e-multi', 'scale');
    await row.getByRole('button', { name: 'Increase replicas' }).click();
    expect((await res).status()).toBe(200);
    await expect(replicas).toHaveText('2');
    await expect.poll(() => countServiceReplicas('e2e-multi', 'cache'), { timeout: 30_000 }).toBe(2);

    res = actionResponse(page, 'e2e-multi', 'scale');
    await row.getByRole('button', { name: 'Decrease replicas' }).click();
    expect((await res).status()).toBe(200);
    await expect(replicas).toHaveText('1');
    await expect.poll(() => countServiceReplicas('e2e-multi', 'cache'), { timeout: 30_000 }).toBe(1);
  });
});
