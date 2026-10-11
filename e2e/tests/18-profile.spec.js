import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

test.describe('Profile', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/profile`);
  });

  test('update display name persists', async ({ page }) => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    try {
      await page.locator('#displayName').fill('Updated Admin Name');
      await page.getByRole('button', { name: 'Save Profile' }).click();
      await expect(page.getByRole('alert').filter({ hasText: 'Profile updated' })).toBeVisible();

      const me = await apiRequest('GET', '/api/me');
      expect(me.data.display_name).toBe('Updated Admin Name');
      expect(me.data.email).toBe(TEST_ADMIN.email);
      await page.reload();
      await expect(page.locator('#displayName')).toHaveValue('Updated Admin Name');
    } finally {
      const restore = await apiRequest('PUT', '/api/me', { display_name: TEST_ADMIN.displayName, email: TEST_ADMIN.email });
      expect(restore.status).toBe(200);
    }
  });

  test('change password: old one stops working, new one signs in', async ({ page }) => {
    const newPassword = 'NewE2ePass456!';
    try {
      await page.locator('#currentPw').fill(TEST_ADMIN.password);
      await page.locator('#newPw').fill(newPassword);
      await page.locator('#confirmPw').fill(newPassword);
      await page.getByRole('button', { name: 'Change Password' }).click();
      await expect(page.getByRole('alert').filter({ hasText: 'Password changed' })).toBeVisible();

      expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(401);
      expect((await apiLogin(TEST_ADMIN.username, newPassword)).status).toBe(200);
    } finally {
      // Later specs sign in with the original password: restore it whichever one is current.
      if ((await apiLogin(TEST_ADMIN.username, newPassword)).status === 200) {
        const res = await apiRequest('PUT', '/api/me/password', { current_password: newPassword, new_password: TEST_ADMIN.password });
        expect(res.status).toBe(200);
      }
      expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    }
  });
});
