import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Endpoints & Access', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /settings/i }).click();
  });

  test('shows current endpoints', async ({ page }) => {
    await expect(page.getByText('nginx-test.local')).toBeVisible({ timeout: 5_000 });
  });

  test('shows TLS mode', async ({ page }) => {
    await expect(page.getByText(/off/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('IP allowlist section visible', async ({ page }) => {
    const advancedBtn = page.getByText(/advanced/i);
    if (await advancedBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await advancedBtn.click();
    }
    await expect(page.locator('#allowlist-input')).toBeVisible({ timeout: 5_000 });
  });
});
