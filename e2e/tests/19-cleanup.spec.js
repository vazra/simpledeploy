import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Cleanup', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('remove postgres app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /settings/i }).click();

    const dangerBtn = page.getByText(/danger|delete|advanced/i).last();
    if (await dangerBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await dangerBtn.click();
    }

    const deleteBtn = page.getByRole('button', { name: /delete app/i });
    await deleteBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 3_000 });
    const confirmInput = dialog.locator('input');
    if (await confirmInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await confirmInput.fill('e2e-postgres');
    }
    await dialog.getByRole('button', { name: /delete|confirm|remove/i }).click();

    await page.waitForSelector('text=Applications', { timeout: 15_000 });
  });

  test('remove multi app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-multi`);
    await page.getByRole('button', { name: /settings/i }).click();

    const dangerBtn = page.getByText(/danger|delete|advanced/i).last();
    if (await dangerBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await dangerBtn.click();
    }

    const deleteBtn = page.getByRole('button', { name: /delete app/i });
    await deleteBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 3_000 });
    const confirmInput = dialog.locator('input');
    if (await confirmInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await confirmInput.fill('e2e-multi');
    }
    await dialog.getByRole('button', { name: /delete|confirm|remove/i }).click();
    await page.waitForSelector('text=Applications', { timeout: 15_000 });
  });

  test('remove nginx app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /settings/i }).click();

    const dangerBtn = page.getByText(/danger|delete|advanced/i).last();
    if (await dangerBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await dangerBtn.click();
    }

    const deleteBtn = page.getByRole('button', { name: /delete app/i });
    await deleteBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 3_000 });
    const confirmInput = dialog.locator('input');
    if (await confirmInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await confirmInput.fill('e2e-nginx');
    }
    await dialog.getByRole('button', { name: /delete|confirm|remove/i }).click();
    await page.waitForSelector('text=Applications', { timeout: 15_000 });
  });

  test('dashboard shows no apps', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await expect(page.getByText('e2e-nginx')).not.toBeVisible({ timeout: 5_000 });
    await expect(page.getByText('e2e-multi')).not.toBeVisible({ timeout: 5_000 });
    await expect(page.getByText('e2e-postgres')).not.toBeVisible({ timeout: 5_000 });
  });
});
