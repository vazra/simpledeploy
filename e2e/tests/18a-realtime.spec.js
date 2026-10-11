import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

// Realtime WS keeps pages in sync without manual refresh. We mutate via a
// separate API session and watch the open page reflect the change.
test.describe('Realtime updates', () => {
  test('Users page reflects new user without refresh', async ({ page }) => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/users`);
    // Users page has settled once the admin row is present.
    await expect(page.locator('main table').getByText(`@${TEST_ADMIN.username}`)).toBeVisible();

    const username = `rt-${Date.now()}`;
    const res = await apiRequest('POST', '/api/users', {
      username,
      password: 'RealtimeTestPass1!',
      role: 'viewer',
      display_name: 'RT User',
      email: 'rt@test.local',
    });
    expect(res.status).toBe(201);
    try {
      // No reload: the realtime store should have refetched the users list.
      await expect(page.locator('main table').getByText(`@${username}`)).toBeVisible({ timeout: 8_000 });
    } finally {
      expect((await apiRequest('DELETE', `/api/users/${res.data.id}`)).status).toBe(200);
    }
  });
});
