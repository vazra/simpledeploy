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

  test('cleanup imported clone app', async () => {
    // Delete via API so 19/99-cleanup specs see a stable state.
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const res = await apiRequest('DELETE', '/api/apps/e2e-nginx-clone');
    // Accept ok or 404 (in case earlier test failed before creating it).
    expect(res.ok || res.status === 404).toBe(true);
  });
});
