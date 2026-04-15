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
    await page.getByRole('button', { name: /disk cleanup/i }).click();
    await expect(page.getByText(/containers|images|volumes|build cache/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('images tab lists images', async ({ page }) => {
    await page.getByRole('button', { name: /images/i }).click();
    await expect(page.getByText(/nginx/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('networks tab lists networks', async ({ page }) => {
    await page.getByRole('button', { name: /networks/i }).click();
    await expect(page.getByText(/bridge|network/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('volumes tab lists volumes', async ({ page }) => {
    await page.getByRole('button', { name: /networks|volumes/i }).click();
    await expect(page.getByText(/volume/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('prune containers', async ({ page }) => {
    await page.getByRole('button', { name: /disk cleanup/i }).click();
    const pruneBtn = page.locator('button').filter({ hasText: /prune$/i }).first();
    if (await pruneBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await pruneBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /prune|confirm/i }).click();
      }
      await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 10_000 });
    }
  });
});
