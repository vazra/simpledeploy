import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Endpoints & Access', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'Settings' }).click();
  });

  test('shows current endpoints', async ({ page }) => {
    await expect(page.getByText('nginx-test.local').first()).toBeVisible({ timeout: 10_000 });
  });

  test('shows TLS mode', async ({ page }) => {
    await expect(page.getByText(/No TLS|Auto TLS|Local CA|Custom TLS/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('IP allowlist section visible', async ({ page }) => {
    // Scroll to bottom of page to find Advanced section
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await page.waitForTimeout(500);
    const advancedBtn = page.locator('h3', { hasText: 'Advanced' });
    await advancedBtn.click();
    await expect(page.locator('#allowlist-input')).toBeVisible({ timeout: 10_000 });
  });
});
