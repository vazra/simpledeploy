import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { getAppId, sqliteExec, sqliteQuery } from '../helpers/db.js';

test.describe('Edge Cases', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test('a real login is recorded in the activity log', async () => {
    const res = await apiRequest('GET', '/api/activity');
    expect(res.status).toBe(200);
    const login = res.data.entries.find((e) => e.action === 'login_succeeded');
    expect(login).toMatchObject({ category: 'auth', actor_name: TEST_ADMIN.username });
  });

  test('POST /api/system/prune/metrics deletes rows older than the cutoff only', async () => {
    // 1d tier: its 400d retention keeps the background rollup away from the seed.
    const appId = getAppId('e2e-nginx');
    const now = Math.floor(Date.now() / 1000);
    const oldTs = now - 3 * 86400;
    const newTs = now - 60;
    sqliteExec(`INSERT INTO metrics (app_id, container_id, ts, tier, cpu_pct, mem_bytes, mem_limit) VALUES
      (${appId}, 'e2e-prune', ${oldTs}, '1d', 1, 1, 4), (${appId}, 'e2e-prune', ${newTs}, '1d', 1, 1, 4);`);
    try {
      const res = await apiRequest('POST', '/api/system/prune/metrics', { tier: '1d', days: 1 });
      expect(res.status).toBe(200);
      expect(res.data.deleted).toBeGreaterThanOrEqual(1);
      const left = sqliteQuery(`SELECT ts FROM metrics WHERE container_id = 'e2e-prune' ORDER BY ts;`);
      expect(left.map((r) => r.ts)).toEqual([newTs]);
    } finally {
      sqliteExec(`DELETE FROM metrics WHERE container_id = 'e2e-prune';`);
    }
  });
});
