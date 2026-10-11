import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { fetchViaProxy } from '../helpers/proxy.js';

test.describe('App Logs', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
  });

  test('log viewer streams a fresh nginx access log line', async ({ page }) => {
    await page.getByRole('button', { name: 'logs', exact: true }).click();
    const marker = `/e2e-log-${Date.now()}`;
    const r = await fetchViaProxy('nginx-test.local', marker);
    expect(r.status).toBe(404);
    // nginx access log -> docker logs -> logs WS -> viewer.
    await expect(page.locator('main').getByText(new RegExp(`"GET ${marker} HTTP/1\\.1" 404`))).toBeVisible();
  });

  test('events tab lists the deploy events from the API', async ({ page }) => {
    const res = await apiRequest('GET', '/api/apps/e2e-nginx/events');
    expect(res.status).toBe(200);
    const actions = res.data.map((e) => e.action);
    expect(actions).toContain('deploy');
    expect(actions.filter((a) => /_(failed|unstable)$/.test(a))).toEqual([]);

    await page.getByRole('button', { name: 'events', exact: true }).click();
    const main = page.locator('main');
    for (const action of new Set(actions)) {
      await expect(main.getByText(action, { exact: true })).toHaveCount(actions.filter((a) => a === action).length);
    }
  });
});
