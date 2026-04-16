import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('App Configuration', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
  });

  test('view compose file in config tab', async ({ page }) => {
    await page.getByRole('button', { name: /settings/i }).click();
    await expect(page.getByText('nginx:alpine')).toBeVisible({ timeout: 5_000 });
  });

  test('environment variables section visible', async ({ page }) => {
    await page.getByRole('button', { name: /settings/i }).click();
    await expect(page.getByText(/environment variables/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByRole('button', { name: /add variable/i })).toBeVisible();
  });
});
