import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, getSessionCookie } from '../helpers/api.js';

// e2e-nginx has two compose versions by now: the 03-deploy fixture
// (nginx:alpine) and the 07-app-config Save & Deploy (nginx:1.27-alpine).
test.describe('Deploy History', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test('versions are newest first and the latest downloads as an attachment', async () => {
    const res = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
    expect(res.status).toBe(200);
    const versions = res.data;
    expect(versions.length).toBeGreaterThanOrEqual(2);
    for (let i = 1; i < versions.length; i++) {
      expect(versions[i - 1].version).toBeGreaterThan(versions[i].version);
      expect(new Date(versions[i - 1].created_at).getTime()).toBeGreaterThanOrEqual(new Date(versions[i].created_at).getTime());
    }
    const latest = versions[0];
    const oldest = versions[versions.length - 1];
    expect(latest.content).toMatch(/image: (\S+\/)?nginx:1\.27-alpine\n/);
    expect(oldest.content).toMatch(/image: (\S+\/)?nginx:alpine\n/);

    const dl = await fetch(`${getState().baseURL}/api/apps/e2e-nginx/versions/${latest.id}/download`, {
      headers: { Cookie: getSessionCookie() },
    });
    expect(dl.status).toBe(200);
    expect(dl.headers.get('content-disposition')).toMatch(/^attachment; filename=/);
    expect(await dl.text()).toBe(latest.content);
  });

  test('settings deploy history lists every stored version', async ({ page }) => {
    const res = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
    expect(res.status).toBe(200);
    const versions = res.data;

    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'settings', exact: true }).click();
    const main = page.locator('main');
    await main.getByRole('button', { name: `Deploy History (${versions.length})` }).click();
    await expect(main.locator('span').filter({ hasText: /^v\d+$/ })).toHaveText(versions.map((v) => `v${v.version}`));
  });
});
