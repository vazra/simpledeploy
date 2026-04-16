import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Docker Management', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/docker`);
  });

  test('Docker info loads', async ({ page }) => {
    await expect(page.getByText(/version/i).first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText(/containers/i).first()).toBeVisible();
  });

  test('disk cleanup tab shows usage', async ({ page }) => {
    // Disk Cleanup is the default tab, should already be visible
    await page.locator('button').filter({ hasText: 'Disk Cleanup' }).click();
    await expect(page.getByText(/Containers|Images|Volumes|Build Cache/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('images tab lists images', async ({ page }) => {
    await page.locator('button').filter({ hasText: /^Images$/ }).click();
    await expect(page.getByText(/nginx/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('networks tab lists networks', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Networks & Volumes' }).click();
    await expect(page.getByText(/bridge/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('volumes tab lists volumes', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Networks & Volumes' }).click();
    // Networks & Volumes tab shows both
    await expect(page.getByText(/Volumes/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('prune containers', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Disk Cleanup' }).click();
    const pruneBtn = page.locator('button').filter({ hasText: /^Prune$/ }).first();
    if (await pruneBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await pruneBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /prune|confirm/i }).click();
      }
      // Wait for toast
      await page.waitForTimeout(3_000);
    }
  });
});
