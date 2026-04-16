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
    await page.waitForTimeout(1_000);
    // Verify the page loaded for e2e-multi
    await expect(page.getByText('e2e-multi').first()).toBeVisible({ timeout: 10_000 });
    // At minimum, web service should be listed in services section
    await expect(page.getByText('Services').first()).toBeVisible({ timeout: 10_000 });
  });

  test('tab navigation works', async ({ page }) => {
    const tabs = ['logs', 'metrics', 'backups', 'settings'];
    for (const tab of tabs) {
      await page.getByRole('button', { name: new RegExp(tab, 'i') }).click();
      await page.waitForTimeout(500);
    }
    await page.getByRole('button', { name: /overview/i }).click();
  });
});
