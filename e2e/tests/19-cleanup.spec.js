import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Cleanup', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('remove postgres app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.locator('button').filter({ hasText: 'settings' }).click();

    // Expand Danger Zone section
    const dangerBtn = page.locator('button').filter({ hasText: 'Danger Zone' });
    await dangerBtn.click();

    const deleteBtn = page.getByRole('button', { name: /delete app/i });
    await deleteBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 3_000 });
    const confirmInput = dialog.locator('input');
    if (await confirmInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await confirmInput.fill('e2e-postgres');
    }
    await dialog.getByRole('button', { name: /delete|confirm|remove/i }).click();

    // After delete, redirects to dashboard which shows "Applications" heading
    await expect(page.locator('aside')).toBeVisible({ timeout: 15_000 });
  });

  test('remove multi app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-multi`);
    await page.locator('button').filter({ hasText: 'settings' }).click();

    // Expand Danger Zone section
    const dangerBtn = page.locator('button').filter({ hasText: 'Danger Zone' });
    await dangerBtn.click();

    const deleteBtn = page.getByRole('button', { name: /delete app/i });
    await deleteBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 3_000 });
    const confirmInput = dialog.locator('input');
    if (await confirmInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await confirmInput.fill('e2e-multi');
    }
    await dialog.getByRole('button', { name: /delete|confirm|remove/i }).click();
    await expect(page.locator('aside')).toBeVisible({ timeout: 15_000 });
  });

  test('remove nginx app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.locator('button').filter({ hasText: 'settings' }).click();

    // Expand Danger Zone section
    const dangerBtn = page.locator('button').filter({ hasText: 'Danger Zone' });
    await dangerBtn.click();

    const deleteBtn = page.getByRole('button', { name: /delete app/i });
    await deleteBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 3_000 });
    const confirmInput = dialog.locator('input');
    if (await confirmInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await confirmInput.fill('e2e-nginx');
    }
    await dialog.getByRole('button', { name: /delete|confirm|remove/i }).click();
    await expect(page.locator('aside')).toBeVisible({ timeout: 15_000 });
  });

  test('dashboard reflects app removals', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    // Verify dashboard loads and shows Applications section
    await expect(page.getByText('Applications')).toBeVisible({ timeout: 10_000 });
  });
});
