import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Endpoints & Access', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    // Click the settings tab button
    await page.locator('button').filter({ hasText: 'settings' }).click();
  });

  test('shows current endpoints', async ({ page }) => {
    // Endpoints section header should be visible in visual mode
    await expect(page.getByText('Endpoints')).toBeVisible({ timeout: 5_000 });
    // Check for endpoint domain or "No domain" fallback
    const hasDomain = await page.getByText('nginx-test.local').isVisible({ timeout: 3_000 }).catch(() => false);
    const hasNoDomain = await page.getByText('No domain').isVisible({ timeout: 1_000 }).catch(() => false);
    expect(hasDomain || hasNoDomain).toBeTruthy();
  });

  test('shows TLS mode', async ({ page }) => {
    // TLS badges use labels like "No TLS", "Auto TLS", "Local CA", "Custom TLS"
    await expect(page.getByText(/No TLS|Auto TLS|Local CA|Custom TLS/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('IP allowlist section visible', async ({ page }) => {
    // Advanced is a collapsible section
    const advancedBtn = page.locator('button').filter({ hasText: 'Advanced' });
    await advancedBtn.click();
    await expect(page.locator('#allowlist-input')).toBeVisible({ timeout: 5_000 });
  });
});
