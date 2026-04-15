import { test, expect } from '@playwright/test';
import { getState, TEST_ADMIN } from '../helpers/auth.js';

test.describe('Initial Setup', () => {
  test('redirects to login when no users exist', async ({ page }) => {
    const state = getState();
    await page.goto(state.baseURL);
    await expect(page).toHaveURL(/login/);
  });

  test('shows setup mode on login page', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await expect(page.getByRole('button', { name: 'Create Account' })).toBeVisible();
  });

  test('rejects empty username', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#password').fill(TEST_ADMIN.password);
    await page.locator('#confirmPassword').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: 'Create Account' }).click();
    await expect(page.getByRole('button', { name: 'Create Account' })).toBeVisible();
  });

  test('rejects short password', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill('short');
    await page.locator('#confirmPassword').fill('short');
    await page.getByRole('button', { name: 'Create Account' }).click();
    await expect(page.getByRole('button', { name: 'Create Account' })).toBeVisible();
  });

  test('rejects mismatched passwords', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill(TEST_ADMIN.password);
    await page.locator('#confirmPassword').fill('DifferentPass123!');
    await page.getByRole('button', { name: 'Create Account' }).click();
    await expect(page.getByText(/match/i)).toBeVisible();
  });

  test('creates admin account successfully', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#displayName').fill(TEST_ADMIN.displayName);
    await page.locator('#email').fill(TEST_ADMIN.email);
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill(TEST_ADMIN.password);
    await page.locator('#confirmPassword').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: 'Create Account' }).click();
    await page.waitForURL(url => url.hash === '#/' || url.hash === '' || !url.hash.includes('login'), { timeout: 10_000 });
  });
});
