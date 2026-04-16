import { test, expect } from '@playwright/test';
import { getState, TEST_ADMIN, login, loginAsAdmin, logout } from '../helpers/auth.js';

test.describe('Authentication', () => {
  test('login with correct credentials', async ({ page }) => {
    await loginAsAdmin(page);
    // Sidebar should be visible after login
    await expect(page.locator('aside')).toBeVisible();
  });

  test('login with wrong password shows error', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill('WrongPassword123!');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText(/invalid|incorrect|wrong/i)).toBeVisible({ timeout: 5_000 });
  });

  test('login with nonexistent user shows error', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#username').fill('nonexistent');
    await page.locator('#password').fill('SomePassword123!');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText(/invalid|incorrect|wrong/i)).toBeVisible({ timeout: 5_000 });
  });

  test('logout and redirect to login', async ({ page }) => {
    await loginAsAdmin(page);
    await logout(page);
    await expect(page).toHaveURL(/login/);
  });

  test('accessing protected route without session redirects to login', async ({ page }) => {
    const state = getState();
    await page.context().clearCookies();
    await page.goto(`${state.baseURL}/#/users`);
    await expect(page).toHaveURL(/login/, { timeout: 5_000 });
  });

  test('re-login after logout works', async ({ page }) => {
    await loginAsAdmin(page);
    await logout(page);
    await loginAsAdmin(page);
    await expect(page.locator('aside')).toBeVisible();
  });
});
