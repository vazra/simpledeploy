import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { fetchViaProxy } from '../helpers/proxy.js';
import { sqliteQuery, getAppId } from '../helpers/db.js';

test.describe('App Metrics', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test('switching to 6h loads container metrics', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'metrics', exact: true }).click();
    const res = page.waitForResponse((r) => r.url().endsWith('/api/apps/e2e-nginx/metrics?range=6h'));
    await page.getByRole('button', { name: '6h', exact: true }).click();
    const metrics = await res;
    expect(metrics.status()).toBe(200);
    expect(Object.keys((await metrics.json()).containers || {}).length).toBeGreaterThan(0);
  });

  test('proxied requests and container stats land in the metrics store', async () => {
    const appId = getAppId('e2e-nginx');
    const since = Math.floor(Date.now() / 1000) - 1;
    for (let i = 0; i < 30; i++) {
      const r = await fetchViaProxy('nginx-test.local', `/?i=${i}`);
      expect(r.status).toBe(200);
    }
    // Request stats flush every 1s in e2e (SIMPLEDEPLOY_REQUEST_METRICS_FLUSH_INTERVAL).
    await expect.poll(() => Number(sqliteQuery(
      `SELECT COALESCE(SUM(count), 0) AS c FROM request_metrics WHERE app_id=${appId} AND tier='raw' AND ts >= ${since}`,
    )[0].c), { timeout: 15_000 }).toBeGreaterThanOrEqual(30);

    const res = await apiRequest('GET', '/api/apps/e2e-nginx/metrics?range=1h');
    expect(res.status).toBe(200);
    const points = Object.values(res.data.containers || {}).flatMap((c) => c.points || []);
    expect(points.some((p) => typeof p.c === 'number' && p.c >= 0)).toBe(true);
  });
});
