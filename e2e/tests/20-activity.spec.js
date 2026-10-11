import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { b64 } from '../helpers/users.js';

async function openAppActivity(page, slug) {
  await page.goto(`${getState().baseURL}/#/apps/${slug}`);
  await page.locator('button').filter({ hasText: /^activity$/ }).first().click();
}

const rows = (page) => page.locator('main [data-testid="activity-row"]');

// Not serial: the version check is expected to fail until the deploy audit
// carries the version, and must not skip the rest. Purge stays last.
test.describe('Activity changelog', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  // The deployer cannot know the version it deploys, so the summary leaves
  // it out instead of claiming "version 0".
  test('per-app deploy entries record successful deploys without a bogus version', async ({ page }) => {
    await openAppActivity(page, 'e2e-nginx');
    await page.locator('[data-testid="activity-filter-deploy"]').click();
    await expect(rows(page).filter({ hasText: 'deploy_succeeded' }).first()).toBeVisible();
    await expect(rows(page).filter({ hasText: /Deploy succeeded/ }).first()).toBeVisible();
    await expect(rows(page).filter({ hasText: /version 0\)/ })).toHaveCount(0);
  });

  test('global audit (system page) shows entries from multiple apps', async ({ page }) => {
    await page.goto(`${getState().baseURL}/#/system`);
    await page.locator('button').filter({ hasText: 'Audit Log' }).click();
    await expect(rows(page).first()).toBeVisible();

    // Filter to app categories so app-scoped rows surface above the login noise.
    await page.locator('[data-testid="activity-filter-lifecycle"]').click();
    await page.locator('[data-testid="activity-filter-compose"]').click();
    await expect.poll(async () => {
      const slugs = await rows(page).locator('a[href^="#/apps/"]').allTextContents();
      return new Set(slugs.map((s) => s.trim()).filter((s) => /^e2e-(nginx|multi|postgres)$/.test(s))).size;
    }).toBeGreaterThanOrEqual(2);
  });

  test('failed deploy entry shows error inline', async ({ page }) => {
    const slug = 'e2e-bad-image';
    // Unreachable local registry: fails fast, no network, not rewritten by the mirror.
    const compose = 'services:\n  web:\n    image: 127.0.0.1:1/e2e/missing:nope\n';
    try {
      const res = await apiRequest('POST', '/api/apps/deploy', { name: slug, compose: b64(compose) });
      expect(res.status).toBe(202);
      await waitForAppStatus(slug, 'error', 90_000);

      let failed;
      await expect.poll(async () => {
        const act = await apiRequest('GET', `/api/apps/${slug}/activity`);
        failed = act.data.entries.find((e) => e.action === 'deploy_failed');
        return failed?.error || '';
      }).not.toBe('');

      await openAppActivity(page, slug);
      const row = rows(page).filter({ hasText: 'deploy_failed' });
      await expect(row).toHaveCount(1);
      await expect(row).toContainText(`Deploy failed: ${failed.error}`);
      await expect(row.locator('.text-red-400, details summary').filter({ hasText: failed.error.slice(0, 100) })).toBeVisible();
      // The inline error is how users learn why a deploy failed, so it must
      // carry docker's reason, not only "compose up: exit status 1". Docker
      // versions word it differently; all name the unreachable registry.
      expect(failed.error).not.toMatch(/^compose up: exit status \d+$/);
      expect(failed.error).toContain('127.0.0.1:1');
    } finally {
      expect((await apiRequest('DELETE', `/api/apps/${slug}`)).status).toBe(200);
    }
  });

  test('env vars edit appears as an env row naming the variable', async ({ page }) => {
    const original = await apiRequest('GET', '/api/apps/e2e-nginx/env');
    expect(original.status).toBe(200);
    try {
      const r = await apiRequest('PUT', '/api/apps/e2e-nginx/env', [...original.data, { key: 'AUDIT_TEST', value: 'hello' }]);
      expect(r.status).toBe(200);
      await openAppActivity(page, 'e2e-nginx');
      await expect(rows(page).filter({ hasText: /Environment variables: added AUDIT_TEST/ }).first()).toBeVisible();
    } finally {
      expect((await apiRequest('PUT', '/api/apps/e2e-nginx/env', original.data)).status).toBe(200);
    }
  });

  test('super-admin can purge all activity', async ({ page }) => {
    await page.goto(`${getState().baseURL}/#/system`);
    await page.locator('button').filter({ hasText: 'Audit Log' }).click();
    await expect(rows(page).first()).toBeVisible();

    await page.getByRole('button', { name: 'Purge all activity' }).click();
    const purgeP = page.waitForResponse((r) => r.request().method() === 'DELETE' && r.url().endsWith('/api/activity'));
    await page.getByRole('button', { name: 'Yes, purge all' }).click();
    expect((await purgeP).status()).toBe(204);

    const after = await apiRequest('GET', '/api/activity?limit=200');
    expect(after.status).toBe(200);
    const entries = after.data.entries;
    expect(entries.filter((e) => ['deploy', 'compose', 'lifecycle', 'env'].includes(e.category))).toEqual([]);
    // The purge leaves a sentinel row naming who did it.
    expect(entries.find((e) => e.action === 'audit_purged')).toMatchObject({ actor_name: TEST_ADMIN.username });
  });
});
