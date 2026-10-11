import { test, expect } from '@playwright/test';
import { rmSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { listAppContainers } from '../helpers/docker.js';
import { fetchViaProxy } from '../helpers/proxy.js';

const archivedSlugs = async () => (await apiRequest('GET', '/api/apps/archived')).data.map((a) => a.Slug);

// Archive lifecycle: removing an app's directory on disk makes the reconciler
// tear the app down and archive it (tombstone); the Archive page lists it and
// "Clean up" purges it.
test.describe('Archive lifecycle', () => {
  test('archives e2e-nginx after dir removal and purges via Clean up', async ({ page }) => {
    const state = getState();
    const slug = 'e2e-nginx';
    await loginAsAdmin(page);
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);

    await expect(page.locator('main').getByRole('heading', { name: slug, exact: true })).toBeVisible();

    rmSync(join(state.appsDir, slug), { recursive: true, force: true });

    await expect.poll(archivedSlugs, { message: 'app archived after dir removal', timeout: 60_000 }).toContain(slug);
    const live = await apiRequest('GET', '/api/apps');
    expect(live.data.map((a) => a.Slug)).not.toContain(slug);

    // Archiving tears the stack down before marking the row archived.
    expect(listAppContainers(slug)).toEqual([]);
    // Caddy answers hosts without a route with an empty 200; a stale route
    // to the removed container would give 502.
    await expect.poll(async () => {
      const r = await fetchViaProxy('nginx-test.local');
      return [r.status, await r.text()];
    }, { message: 'proxy route removed', timeout: 15_000 }).toEqual([200, '']);

    await page.reload();
    await expect(page.locator('main').getByRole('heading', { name: slug, exact: true })).toHaveCount(0);

    await page.goto(`${state.baseURL}/#/archive`);
    const main = page.locator('main');
    const row = main.locator('tr', { hasText: slug });
    await expect(row).toHaveCount(1);
    await row.getByRole('button', { name: 'Clean up' }).click();

    const dialog = page.getByRole('dialog');
    const purged = page.waitForResponse((r) => r.url().endsWith(`/api/apps/${slug}/purge`));
    await dialog.getByRole('button', { name: 'Confirm' }).click();
    expect((await purged).status()).toBe(204);

    await expect(main.locator('tr', { hasText: slug })).toHaveCount(0);
    expect(await archivedSlugs()).not.toContain(slug);
  });
});
