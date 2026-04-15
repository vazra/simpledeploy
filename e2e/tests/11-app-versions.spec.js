import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Deploy History & Rollback', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /settings/i }).click();
  });

  test('shows deploy history', async ({ page }) => {
    const historySection = page.getByText(/deploy history|versions/i).first();
    await expect(historySection).toBeVisible({ timeout: 5_000 });
  });

  test('deploy history has entries', async ({ page }) => {
    const versionEntries = page.getByText(/v\d|version|#\d/i);
    await expect(versionEntries.first()).toBeVisible({ timeout: 5_000 });
  });
});
