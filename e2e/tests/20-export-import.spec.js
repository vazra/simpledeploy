import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { dockerCLI } from '../helpers/docker.js';
import { latestDeployEventId, waitForDeployEvent } from '../helpers/users.js';

test.describe.configure({ mode: 'serial' });

// e2e-postgres publishes no host port and claims no domain, so a clone of it
// can run next to the original.
const SOURCE = 'e2e-postgres';
const CLONE = 'e2e-postgres-clone';

async function openImportModal(page, bundlePath) {
  await page.goto(`${getState().baseURL}/#/`);
  await page.getByRole('button', { name: 'Deploy App' }).first().click();
  await expect(page.getByRole('dialog').first()).toBeVisible();
  await page.getByTestId('wizard-import-btn').click();
  const importModal = page.locator('[role="dialog"][aria-modal="true"]').last();
  await page.getByTestId('import-file').setInputFiles(bundlePath);
  return importModal;
}

test.describe('App Export/Import', () => {
  let bundlePath;

  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const res = await apiRequest('DELETE', `/api/apps/${CLONE}`);
    expect([200, 404]).toContain(res.status);
    dockerCLI(['volume', 'rm', '-f', `simpledeploy-${CLONE}_pgdata`]);
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('export an app and re-import it as a new running app', async ({ page }) => {
    await page.goto(`${getState().baseURL}/#/apps/${SOURCE}`);
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
    expect(download.suggestedFilename()).toBe(`${SOURCE}.simpledeploy.zip`);
    bundlePath = await download.path();
    const buf = readFileSync(bundlePath);
    expect(buf.subarray(0, 4)).toEqual(Buffer.from([0x50, 0x4b, 0x03, 0x04]));

    const importModal = await openImportModal(page, bundlePath);
    await page.getByTestId('import-slug').fill(CLONE);
    // Mode defaults to "new".
    await importModal.getByRole('button', { name: /^import$/i }).click();
    await page.waitForURL(new RegExp(`#/apps/${CLONE}`), { timeout: 60_000 });

    // The clone runs the same compose as its source.
    const ev = await waitForDeployEvent(CLONE, 0);
    expect(ev.action).toBe('deploy');
    await waitForAppStatus(CLONE, 'running');
    const src = await apiRequest('GET', `/api/apps/${SOURCE}/compose`);
    const clone = await apiRequest('GET', `/api/apps/${CLONE}/compose`);
    expect(clone.status).toBe(200);
    expect(clone.data).toBe(src.data);
  });

  test('overwrite-mode import shows preview and redeploys on confirm', async ({ page }) => {
    const importModal = await openImportModal(page, bundlePath);
    await importModal.getByLabel('Overwrite existing app').check();
    await page.getByTestId('import-slug').fill(SOURCE);
    await importModal.getByRole('button', { name: /^import$/i }).click();

    // Re-import of an unchanged export -> Compose Unchanged, Sidecar Unchanged.
    await expect(page.getByTestId('import-preview')).toBeVisible();
    await expect(page.getByTestId('import-preview-compose')).toContainText('Unchanged');
    await expect(page.getByTestId('import-preview-sidecar')).toContainText('Unchanged');
    await expect(page.getByTestId('import-preview-alerts')).toBeVisible();
    await expect(page.getByTestId('import-preview-backups')).toBeVisible();

    const before = await latestDeployEventId(SOURCE);
    await importModal.getByRole('button', { name: /confirm overwrite/i }).click();
    await page.waitForURL(new RegExp(`#/apps/${SOURCE}(\\?|$|/)`), { timeout: 30_000 });
    await expect(importModal).toBeHidden();

    // Later specs use e2e-postgres: wait for the redeploy to finish.
    expect((await waitForDeployEvent(SOURCE, before)).action).toBe('deploy');
    await waitForAppStatus(SOURCE, 'running');
  });
});
