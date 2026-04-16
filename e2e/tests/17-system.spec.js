import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('System Administration', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/system`);
  });

  test('system overview loads', async ({ page }) => {
    // Section heading "SimpleDeploy" in overview tab
    await expect(page.locator('h2').filter({ hasText: /SimpleDeploy/i }).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/uptime/i).first()).toBeVisible();
  });

  test('shows database info', async ({ page }) => {
    await expect(page.getByText(/database|sqlite/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('shows system resources', async ({ page }) => {
    await expect(page.getByText(/cpu|cores/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/ram|memory/i).first()).toBeVisible();
  });

  test('maintenance tab - vacuum database', async ({ page }) => {
    // Tab buttons are not role="button" by default in this UI; use text-based selector
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    const vacuumBtn = page.getByRole('button', { name: /Run VACUUM/i });
    await expect(vacuumBtn).toBeVisible({ timeout: 5_000 });
    await vacuumBtn.click();
    // Wait for toast or success indication
    await page.waitForTimeout(3_000);
  });

  test('maintenance tab - prune metrics', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    // "Prune Metrics" section has a select for tiers
    const tierSelect = page.locator('select').first();
    await expect(tierSelect).toBeVisible({ timeout: 5_000 });
  });

  test('maintenance tab - download database backup', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    const downloadBtn = page.getByRole('button', { name: /Download Now/i });
    await expect(downloadBtn).toBeVisible({ timeout: 5_000 });
  });

  test('audit log tab loads', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Audit Log' }).click();
    // Audit log section has heading "Audit Log" and table headers or empty state
    await expect(page.getByText(/Security events|Time|No audit events/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('logs tab loads', async ({ page }) => {
    await page.locator('button').filter({ hasText: /^Logs$/ }).click();
    // Logs tab has "Auto-scroll" checkbox label and "Refresh" button
    await expect(page.getByText(/Auto-scroll/i).first()).toBeVisible({ timeout: 5_000 });
  });
});
