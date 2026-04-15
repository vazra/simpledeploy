import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('App Metrics', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /metrics/i }).click();
  });

  test('metrics charts render', async ({ page }) => {
    const canvases = page.locator('canvas');
    await expect(canvases.first()).toBeVisible({ timeout: 10_000 });
  });

  test('time range buttons visible', async ({ page }) => {
    await expect(page.getByRole('button', { name: '1h' })).toBeVisible();
    await expect(page.getByRole('button', { name: '24h' })).toBeVisible();
  });

  test('switch time range', async ({ page }) => {
    await page.getByRole('button', { name: '6h' }).click();
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 10_000 });
  });

  test('CPU and memory chart labels visible', async ({ page }) => {
    await expect(page.getByText(/cpu/i).first()).toBeVisible();
    await expect(page.getByText(/memory/i).first()).toBeVisible();
  });
});
