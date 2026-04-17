import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { fetchViaProxy } from '../helpers/proxy.js';
import { sqliteQuery } from '../helpers/db.js';

test.describe('App Metrics', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /metrics/i }).click();
  });

  test('metrics charts render', async ({ page }) => {
    // Charts need time to fetch data and render via Chart.js
    const canvases = page.locator('canvas');
    await expect(canvases.first()).toBeVisible({ timeout: 15_000 });
  });

  test('time range buttons visible', async ({ page }) => {
    await expect(page.getByRole('button', { name: '1h' })).toBeVisible();
    await expect(page.getByRole('button', { name: '24h' })).toBeVisible();
  });

  test('switch time range', async ({ page }) => {
    await page.getByRole('button', { name: '6h' }).click();
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 15_000 });
  });

  test('CPU and memory chart labels visible', async ({ page }) => {
    await expect(page.getByText(/cpu/i).first()).toBeVisible();
    await expect(page.getByText(/memory/i).first()).toBeVisible();
  });
});

test.describe.configure({ mode: 'serial' });
test.describe('Metrics - Functional', () => {
  test.beforeAll(async () => {
    await apiLogin('e2eadmin', 'E2eTestPass123!');
  });

  test('metrics DB accumulates data after traffic', async () => {
    test.setTimeout(180_000);

    // Generate HTTP load through the proxy.
    for (let i = 0; i < 30; i++) {
      try {
        await fetchViaProxy('nginx-test.local', `/?i=${i}`);
      } catch {}
    }

    // Wait for metrics collection interval + request metrics flush.
    await new Promise((r) => setTimeout(r, 45_000));

    const rows = sqliteQuery(
      "SELECT COUNT(*) AS c FROM metrics WHERE app_id=(SELECT id FROM apps WHERE slug='e2e-nginx')",
    );
    expect(rows.length).toBeGreaterThan(0);
    const c = Number(rows[0].c);
    expect(c).toBeGreaterThan(0);

    const res = await apiRequest('GET', '/api/apps/e2e-nginx/metrics?range=1h');
    expect(res.ok).toBe(true);
    expect(res.data).toBeTruthy();
    expect(res.data.containers).toBeTruthy();
    const cids = Object.keys(res.data.containers || {});
    expect(cids.length).toBeGreaterThan(0);
    let sawCpu = false;
    for (const cid of cids) {
      const pts = res.data.containers[cid].points || [];
      for (const p of pts) {
        if (typeof p.c === 'number' && p.c >= 0) {
          sawCpu = true;
          break;
        }
      }
      if (sawCpu) break;
    }
    expect(sawCpu).toBe(true);

    const reqRows = sqliteQuery(
      "SELECT COUNT(*) AS c FROM request_metrics WHERE app_id=(SELECT id FROM apps WHERE slug='e2e-nginx')",
    );
    expect(reqRows.length).toBeGreaterThan(0);
    expect(Number(reqRows[0].c)).toBeGreaterThan(0);
  });
});
