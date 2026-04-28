import { test, expect } from '@playwright/test';
import { rmSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

// Verifies the archive lifecycle: removing an app's directory on disk causes
// the reconciler to archive it (tombstone), the Archive page lists it, and
// "Clean up" purges the row.
test.describe('Archive lifecycle', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('archives e2e-nginx after dir removal and purges via Clean up', async ({ page }) => {
    const state = getState();
    const slug = 'e2e-nginx';

    // Sanity: app currently visible on dashboard.
    await page.goto(`${state.baseURL}/#/`);
    await expect(
      page.locator('main').getByRole('heading', { name: slug, exact: true })
    ).toBeVisible({ timeout: 15_000 });

    // Remove the app dir on disk to trigger reconciler archive.
    rmSync(join(state.appsDir, slug), { recursive: true, force: true });

    // Poll the API until the reconciler archives the app (debounce + teardown
    // can take several seconds).
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const deadline = Date.now() + 60_000;
    let archived = false;
    while (Date.now() < deadline) {
      const res = await apiRequest('GET', '/api/apps/archived');
      const list = (res.data || []);
      if (list.some((a) => (a.slug || a.Slug) === slug)) { archived = true; break; }
      await page.waitForTimeout(1_000);
    }
    expect(archived, 'app should be archived after dir removal').toBe(true);

    // Confirm the active app list also excludes it.
    const liveList = await apiRequest('GET', '/api/apps');
    const liveSlugs = (liveList.data || []).map((a) => a.slug || a.Slug);
    expect(liveSlugs).not.toContain(slug);

    // Dashboard should no longer list it as an app card. Use the heading
    // selector to avoid matching activity-feed links to the same slug.
    await page.goto(`${state.baseURL}/#/`);
    await page.reload();
    await page.waitForLoadState('networkidle').catch(() => {});
    await expect(
      page.locator('main').getByRole('heading', { name: slug, exact: true })
    ).toHaveCount(0, { timeout: 20_000 });

    // Archive page should list it.
    await page.goto(`${state.baseURL}/#/archive`);
    const main = page.locator('main');
    await expect(main.getByText(slug, { exact: true }).first()).toBeVisible({ timeout: 15_000 });

    // Click Clean up on the row, confirm in the modal.
    const row = main.locator('tr', { hasText: slug }).first();
    await row.getByRole('button', { name: /clean up/i }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /confirm/i }).click();

    // Row should disappear from the archive page.
    await expect(main.getByText(slug, { exact: true })).toHaveCount(0, { timeout: 15_000 });
  });
});
