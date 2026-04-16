import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Deploy History & Rollback', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.locator('button').filter({ hasText: 'settings' }).click();
  });

  test('shows deploy history', async ({ page }) => {
    // Deploy History is a collapsible section with text "Deploy History (N)"
    // It only appears if versions > 0; wait for config tab to finish loading
    const historyBtn = page.locator('button').filter({ hasText: /Deploy History/i });
    await expect(historyBtn).toBeVisible({ timeout: 10_000 });
  });

  test('deploy history has entries', async ({ page }) => {
    // Expand the deploy history section
    const historyBtn = page.locator('button').filter({ hasText: /Deploy History/i });
    await historyBtn.click();
    // Entries display as "v1", "v2", etc. in table cells
    await expect(page.locator('td').filter({ hasText: /^v\d+$/ }).first()).toBeVisible({ timeout: 5_000 });
  });
});
