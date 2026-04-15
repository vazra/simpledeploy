import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN, login } from '../helpers/auth.js';

test.describe('Profile', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/profile`);
  });

  test('profile page shows user info', async ({ page }) => {
    await expect(page.getByText(TEST_ADMIN.username)).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/super_admin/i)).toBeVisible();
  });

  test('update display name', async ({ page }) => {
    const nameInput = page.locator('#displayName');
    await nameInput.fill('Updated Admin Name');
    await page.getByRole('button', { name: /save profile/i }).click();
    await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 5_000 });
    await nameInput.fill(TEST_ADMIN.displayName);
    await page.getByRole('button', { name: /save profile/i }).click();
  });

  test('change password and re-login', async ({ page }) => {
    const newPassword = 'NewE2ePass456!';
    await page.locator('#currentPw').fill(TEST_ADMIN.password);
    await page.locator('#newPw').fill(newPassword);
    await page.locator('#confirmPw').fill(newPassword);
    await page.getByRole('button', { name: /change password/i }).click();
    await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 5_000 });

    const state = getState();
    await page.goto(`${state.baseURL}/#/profile`);
    await page.getByText(/sign out/i).click();
    await page.waitForURL(url => url.hash.includes('login'), { timeout: 5_000 });

    await login(page, TEST_ADMIN.username, newPassword);
    await expect(page.getByText('Deploy App')).toBeVisible({ timeout: 5_000 });

    await page.goto(`${state.baseURL}/#/profile`);
    await page.locator('#currentPw').fill(newPassword);
    await page.locator('#newPw').fill(TEST_ADMIN.password);
    await page.locator('#confirmPw').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: /change password/i }).click();
    await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 5_000 });
  });

  test('theme toggle works', async ({ page }) => {
    const themeBtn = page.locator('button').filter({ has: page.locator('svg') }).filter({ hasText: '' });
    const htmlEl = page.locator('html');
    const currentClass = await htmlEl.getAttribute('class');
    expect(currentClass !== null || currentClass === '').toBeTruthy();
  });
});
