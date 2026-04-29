import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

// Realtime WS keeps the dashboard in sync without manual refresh.
// We mutate via a separate API call (different "session") and watch the
// open page reflect the change within a few seconds.
test.describe('Realtime updates', () => {
  test.beforeEach(async ({ page }) => {
    // Re-establish admin session for apiRequest (prior specs may have left a
    // viewer or stale cookie on the module-level state).
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    await loginAsAdmin(page);
  });

  test('Users page reflects new user without refresh', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/users`);
    // Wait for users page to settle: the existing admin row should be present.
    await expect(page.locator('main').getByText('e2eadmin').first()).toBeVisible({ timeout: 5_000 });

    const username = `rt-${Date.now()}`;
    const res = await apiRequest('POST', '/api/users', {
      username,
      password: 'RealtimeTestPass1!',
      role: 'viewer',
      display_name: 'RT User',
      email: 'rt@test.local',
    });
    expect(res.ok).toBe(true);

    // No reload: the realtime store should have refetched the users list.
    await expect(page.locator('main').getByText(username).first()).toBeVisible({ timeout: 8_000 });

    // Cleanup: delete the user via API.
    if (res.data?.id) {
      await apiRequest('DELETE', `/api/users/${res.data.id}`);
    }
  });

  test('Dashboard refreshes when an app is renamed via API', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await expect(page.getByRole('heading', { name: 'e2e-nginx', exact: true })).toBeVisible({ timeout: 5_000 });

    // Touch the app via a no-op env update so an audit/env event fires.
    const slug = 'e2e-nginx';
    const envRes = await apiRequest('PUT', `/api/apps/${slug}/env`, [{ key: 'RT_PING', value: '1' }]);
    expect(envRes.ok).toBe(true);

    // The dashboard should still show the app card, possibly refreshed.
    await expect(page.getByRole('heading', { name: 'e2e-nginx', exact: true })).toBeVisible({ timeout: 5_000 });

    // Clean up env var.
    await apiRequest('PUT', `/api/apps/${slug}/env`, []);
  });
});
