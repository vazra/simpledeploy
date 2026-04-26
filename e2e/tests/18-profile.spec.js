import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN, login } from '../helpers/auth.js';

test.describe('Profile', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/profile`);
  });

  test('profile page shows user info', async ({ page }) => {
    // Username appears in sidebar + profile page; scope to main content
    await expect(page.locator('main').getByText(TEST_ADMIN.username).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('main').getByText(/super_admin/i).first()).toBeVisible();
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
    // Click the "Log out" button/link in profile page
    await page.locator('button, a').filter({ hasText: /log out/i }).click();
    await page.waitForSelector('#username', { timeout: 5_000 });

    await login(page, TEST_ADMIN.username, newPassword);
    // After login, sidebar should be visible
    await expect(page.locator('aside:not([data-testid="activity-sidebar"])')).toBeVisible({ timeout: 5_000 });

    await page.goto(`${state.baseURL}/#/profile`);
    await page.locator('#currentPw').fill(newPassword);
    await page.locator('#newPw').fill(TEST_ADMIN.password);
    await page.locator('#confirmPw').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: /change password/i }).click();
    await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 5_000 });
  });

  test('theme toggle works', async ({ page }) => {
    const themeBtn = page.getByRole('button', { name: 'Toggle theme' });
    await expect(themeBtn).toBeVisible();
    // Get initial title (e.g., "Theme: system")
    const titleBefore = await themeBtn.getAttribute('title');
    // Click to cycle theme
    await themeBtn.click();
    // Title should change (system -> light -> dark)
    const titleAfter = await themeBtn.getAttribute('title');
    expect(titleAfter).not.toBe(titleBefore);
    // Click again to restore for other tests
    await themeBtn.click();
  });
});
