import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('App Actions', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('stop app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /stop/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const confirmBtn = dialog.getByRole('button', { name: /stop|confirm|yes/i });
      if (await confirmBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await confirmBtn.click();
      }
    }
    await expect(page.getByText(/stopped/i).first()).toBeVisible({ timeout: 30_000 });
  });

  test('start app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    // App may be running or stopped; click Start if available
    const startBtn = page.getByRole('button', { name: /^start$/i });
    if (await startBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await startBtn.click();
    }
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 30_000 });
  });

  test('restart app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    // Wait for page to load and check app state
    await page.waitForTimeout(2_000);
    // If app is stopped, start it first so Restart button appears
    const startBtn = page.getByRole('button', { name: 'Start' });
    if (await startBtn.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await startBtn.click();
      // Wait for action to complete and status to change
      await page.waitForTimeout(5_000);
      await page.reload();
      await page.waitForTimeout(2_000);
    }
    await page.getByRole('button', { name: /restart/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const confirmBtn = dialog.getByRole('button', { name: /restart|confirm/i });
      if (await confirmBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await confirmBtn.click();
      }
    }
    // Wait for action modal close button (inside the dialog, not the backdrop)
    const actionDialog = page.getByRole('dialog');
    const closeBtn = actionDialog.locator('button:has-text("Close"):not([aria-label])');
    await expect(closeBtn).toBeVisible({ timeout: 60_000 });
    await closeBtn.click();
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 15_000 });
  });

  test('pull and update', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /pull/i }).click();
    // Wait for action modal close button (inside dialog footer, not backdrop)
    const actionDialog = page.getByRole('dialog');
    const closeBtn = actionDialog.locator('button:has-text("Close"):not([aria-label])');
    await expect(closeBtn).toBeVisible({ timeout: 120_000 });
    await closeBtn.click();
  });

  test('scale service', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-multi`);
    // Scale is inside the "..." (more) dropdown menu
    // The more button is a ghost button containing only an SVG with three dots
    const moreBtn = page.locator('button').filter({ has: page.locator('svg path[d*="6.75 12a.75"]') });
    await moreBtn.click();
    // Click "Scale" in the dropdown
    await page.locator('button').filter({ hasText: 'Scale' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    const inputs = dialog.locator('input[type="number"]');
    const count = await inputs.count();
    if (count > 0) {
      await inputs.first().fill('2');
    }
    const applyBtn = dialog.getByRole('button', { name: /apply/i });
    await applyBtn.click();
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 30_000 });
  });
});
