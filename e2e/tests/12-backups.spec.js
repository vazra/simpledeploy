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

  test('detect strategies shows postgres and volume', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
    await configBtn.click();
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5_000 });

    // Strategy detection should show PostgreSQL and Files & Volumes
    await expect(page.getByText(/postgresql/i)).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/files.*volumes/i)).toBeVisible();
  });

  test('create backup config via wizard', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
    await configBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Step 1: Select postgres strategy (auto-detected, may already be selected)
    const pgBtn = dialog.getByText(/postgresql/i).first();
    if (await pgBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await pgBtn.click();
    }
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 2: Select local storage (default)
    await dialog.getByText(/local storage/i).click();
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 3: Schedule (accept defaults)
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 4: Hooks (skip)
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 5: Retention (accept defaults)
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 6: Review and create
    await dialog.getByRole('button', { name: /create backup/i }).click();

    // Verify config appears in the table
    await expect(page.getByText(/local/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('trigger manual backup', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const backupBtn = page.getByRole('button', { name: /backup now/i });
    await expect(backupBtn).toBeVisible({ timeout: 10_000 });
    await backupBtn.click();

    // Wait for backup to run, then reload to see result
    await page.waitForTimeout(3_000);
    await page.reload();
    await page.getByRole('button', { name: /backups/i }).click();
    await expect(page.getByText(/running|success|failed/i).first()).toBeVisible({ timeout: 15_000 });
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
