import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Backups', () => {
  test('navigate to postgres app backups tab', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();
    await expect(page.getByText(/backup|configure/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('create backup config via wizard', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const addBtn = page.getByRole('button', { name: /configure|add config/i });
    await addBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const strategyBtn = dialog.locator('button').filter({ hasText: /postgres|database|volume/i }).first();
    if (await strategyBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await strategyBtn.click();
    }
    await dialog.getByRole('button', { name: /next/i }).click();

    await dialog.getByText(/local storage/i).click();
    await dialog.getByRole('button', { name: /next/i }).click();

    await dialog.getByRole('button', { name: /next/i }).click();

    await dialog.getByRole('button', { name: /create backup/i }).click();

    await expect(page.getByText(/local|storage/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('trigger manual backup', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const backupBtn = page.getByRole('button', { name: /backup now/i });
    await expect(backupBtn).toBeVisible({ timeout: 10_000 });
    await backupBtn.click();

    // Backup runs async; poll by reloading until status changes from "running"
    const deadline = Date.now() + 90_000;
    let found = false;
    while (Date.now() < deadline) {
      await page.waitForTimeout(5_000);
      await page.reload();
      await page.getByRole('button', { name: /backups/i }).click();
      const status = await page.getByText(/success|failed/i).first().isVisible({ timeout: 3_000 }).catch(() => false);
      if (status) { found = true; break; }
    }
    expect(found).toBeTruthy();
  });

  test('global backups page shows summary', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/backups`);
    await expect(page.getByText(/total config/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('delete backup config', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const deleteBtn = page.getByRole('button', { name: /delete/i }).first();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });
});
