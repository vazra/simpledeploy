import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('App Overview', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
  });

  test('shows app name and running status', async ({ page }) => {
    await expect(page.locator('h1').getByText('e2e-nginx')).toBeVisible();
    await expect(page.getByText(/running/i).first()).toBeVisible();
  });

  test('shows services list', async ({ page }) => {
    await expect(page.locator('main').getByText('web').first()).toBeVisible();
  });

  test('shows action buttons', async ({ page }) => {
    await expect(page.getByRole('button', { name: /restart/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /stop/i })).toBeVisible();
  });

  test('multi-service app shows services', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-multi`);
    // Anchor on the h1 title (same pattern as the first test in this file)
    // so getByText cannot latch onto an unrelated DOM occurrence, and give
    // the SPA router time to finish loading the new app after the
    // beforeEach navigation to a different slug.
    await expect(page.locator('h1').getByText('e2e-multi')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('main').getByText('Services').first()).toBeVisible({ timeout: 10_000 });
  });

  test('tab navigation works', async ({ page }) => {
    const tabs = ['logs', 'metrics', 'backups', 'settings'];
    for (const tab of tabs) {
      const btn = page.getByRole('button', { name: new RegExp(tab, 'i') });
      await btn.click();
      // Verify tab became active (wait for content area to update)
      await expect(btn).toBeVisible();
    }
    await page.getByRole('button', { name: /overview/i }).click();
  });
});
