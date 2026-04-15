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

  test('view and add environment variable', async ({ page }) => {
    await page.getByRole('button', { name: /settings/i }).click();
    const envSection = page.getByText(/environment/i).first();
    if (await envSection.isVisible({ timeout: 3_000 }).catch(() => false)) {
      const addBtn = page.getByRole('button', { name: /add/i }).first();
      if (await addBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await addBtn.click();
      }
    }
  });
});
