import { test, expect } from '@playwright/test';
import { readFileSync, existsSync } from 'fs';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

test.describe.configure({ mode: 'serial' });

test.describe('App Export/Import', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('export e2e-nginx config and re-import as a new app', async ({ page }) => {
    const state = getState();

    // 1) Navigate to the e2e-nginx app and open Settings tab.
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /settings/i }).click();

    // 2) Trigger the Export button in the "Export config" section and capture the download.
    const exportSection = page
      .locator('main')
      .locator('div', { hasText: 'Export config' })
      .filter({ has: page.getByRole('button', { name: /^export$/i }) })
      .last();
    const exportBtn = exportSection.getByRole('button', { name: /^export$/i });
    await expect(exportBtn).toBeVisible({ timeout: 10_000 });

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      exportBtn.click(),
    ]);

    expect(download.suggestedFilename()).toBe('e2e-nginx.simpledeploy.zip');
    const downloadPath = await download.path();
    expect(downloadPath).toBeTruthy();
    expect(existsSync(downloadPath)).toBe(true);
    const buf = readFileSync(downloadPath);
    expect(buf.length).toBeGreaterThanOrEqual(4);
    // ZIP magic bytes: 'PK\x03\x04'.
    expect(buf[0]).toBe(0x50); // P
    expect(buf[1]).toBe(0x4b); // K

    // 3) Open Deploy wizard and choose Import from file.
    await page.goto(`${state.baseURL}/#/`);
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const wizard = page.getByRole('dialog').first();
    await expect(wizard).toBeVisible();
    await page.getByTestId('wizard-import-btn').click();

    // The ImportAppModal opens (separate role=dialog).
    const importModal = page.locator('[role="dialog"][aria-modal="true"]').last();
    await expect(importModal).toBeVisible();

    // Upload the zip and fill new slug.
    await page.getByTestId('import-file').setInputFiles(downloadPath);
    await page.getByTestId('import-slug').fill('e2e-nginx-clone');

    // Mode defaults to "new" - click Import.
    await importModal.getByRole('button', { name: /^import$/i }).click();

    // 4) Wait for navigation to the new app page.
    await page.waitForURL(/#\/apps\/e2e-nginx-clone/, { timeout: 60_000 });

    // 5) Confirm the new app appears on the dashboard listing.
    await page.goto(`${state.baseURL}/#/`);
    await expect(page.locator('main').getByText('e2e-nginx-clone').first()).toBeVisible({
      timeout: 15_000,
    });
  });

  test('overwrite-mode import shows preview and applies on confirm', async ({ page }) => {
    const state = getState();

    // 1) Export the existing e2e-nginx app to get a fresh bundle.
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /settings/i }).click();
    const exportSection = page
      .locator('main')
      .locator('div', { hasText: 'Export config' })
      .filter({ has: page.getByRole('button', { name: /^export$/i }) })
      .last();
    const exportBtn = exportSection.getByRole('button', { name: /^export$/i });
    await expect(exportBtn).toBeVisible({ timeout: 10_000 });
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      exportBtn.click(),
    ]);
    const downloadPath = await download.path();
    expect(downloadPath).toBeTruthy();

    // 2) Open Deploy wizard, choose Import from file.
    await page.goto(`${state.baseURL}/#/`);
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    await expect(page.getByRole('dialog').first()).toBeVisible();
    await page.getByTestId('wizard-import-btn').click();

    const importModal = page.locator('[role="dialog"][aria-modal="true"]').last();
    await expect(importModal).toBeVisible();

    // 3) Upload, set overwrite mode, target slug = e2e-nginx.
    await page.getByTestId('import-file').setInputFiles(downloadPath);
    await importModal.getByLabel('Overwrite existing app').check();
    await page.getByTestId('import-slug').fill('e2e-nginx');

    // 4) Click Import -> preview panel appears.
    await importModal.getByRole('button', { name: /^import$/i }).click();
    const preview = page.getByTestId('import-preview');
    await expect(preview).toBeVisible({ timeout: 15_000 });

    // Re-export of unchanged app -> Compose Unchanged, Sidecar Unchanged.
    await expect(page.getByTestId('import-preview-compose')).toContainText('Unchanged');
    await expect(page.getByTestId('import-preview-sidecar')).toContainText('Unchanged');
    await expect(page.getByTestId('import-preview-alerts')).toBeVisible();
    await expect(page.getByTestId('import-preview-backups')).toBeVisible();

    // 5) Confirm overwrite.
    await importModal.getByRole('button', { name: /confirm overwrite/i }).click();

    // Modal should close and we should land on the app page.
    await page.waitForURL(/#\/apps\/e2e-nginx(\?|$|\/)/, { timeout: 30_000 });
    await expect(importModal).toBeHidden();

    // 6) Dashboard still shows e2e-nginx.
    await page.goto(`${state.baseURL}/#/`);
    await expect(page.locator('main').getByText('e2e-nginx', { exact: true }).first()).toBeVisible({
      timeout: 15_000,
    });
  });

  test('overwrite-mode import against missing slug surfaces an error', async ({ page }) => {
    const state = getState();

    // Re-export e2e-nginx to get a bundle.
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /settings/i }).click();
    const exportSection = page
      .locator('main')
      .locator('div', { hasText: 'Export config' })
      .filter({ has: page.getByRole('button', { name: /^export$/i }) })
      .last();
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      exportSection.getByRole('button', { name: /^export$/i }).click(),
    ]);
    const downloadPath = await download.path();

    await page.goto(`${state.baseURL}/#/`);
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    await expect(page.getByRole('dialog').first()).toBeVisible();
    await page.getByTestId('wizard-import-btn').click();

    const importModal = page.locator('[role="dialog"][aria-modal="true"]').last();
    await expect(importModal).toBeVisible();

    await page.getByTestId('import-file').setInputFiles(downloadPath);
    await importModal.getByLabel('Overwrite existing app').check();
    await page.getByTestId('import-slug').fill('e2e-nginx-does-not-exist');
    await importModal.getByRole('button', { name: /^import$/i }).click();

    // Either the preview returned an error (modal stays on form with error)
    // or the preview opened with all-changed rows. Implementation returns 404,
    // so the error is surfaced and the preview panel does NOT render.
    const errorMsg = page.getByTestId('import-error');
    await expect(errorMsg).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId('import-preview')).toBeHidden();

    // Close modal cleanly. Cancel closes ImportAppModal; verify its file input is gone.
    await importModal.getByRole('button', { name: /cancel/i }).click();
    await expect(page.getByTestId('import-file')).toBeHidden();
  });

  test('cleanup imported clone app', async () => {
    // Delete via API so 19/99-cleanup specs see a stable state.
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const res = await apiRequest('DELETE', '/api/apps/e2e-nginx-clone');
    // Accept ok or 404 (in case earlier test failed before creating it).
    expect(res.ok || res.status === 404).toBe(true);
  });
});
