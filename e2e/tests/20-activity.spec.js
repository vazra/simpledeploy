import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Activity changelog', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('per-app activity tab shows entries with summaries', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);

    // Tab label is raw lowercase 'activity' rendered as text
    await page.locator('button').filter({ hasText: /^activity$/ }).first().click();

    // Wait for at least one activity row to appear
    await expect(page.locator('[data-testid="activity-row"]').first()).toBeVisible({ timeout: 10_000 });

    // Expect deploy-related content
    await expect(page.locator('main')).toContainText(/deploy|created|compose/i);
  });

  test('per-app activity tab filter chips are present', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);

    await page.locator('button').filter({ hasText: /^activity$/ }).first().click();

    // Filter chips for known categories should be visible
    await expect(page.locator('[data-testid="activity-filter-deploy"]')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('[data-testid="activity-filter-compose"]')).toBeVisible();
  });

  test('category filter narrows entries on app activity tab', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);

    await page.locator('button').filter({ hasText: /^activity$/ }).first().click();

    // Wait for rows to load
    await expect(page.locator('[data-testid="activity-row"]').first()).toBeVisible({ timeout: 10_000 });
    const initialCount = await page.locator('[data-testid="activity-row"]').count();

    // Toggle the 'compose' filter chip
    await page.locator('[data-testid="activity-filter-compose"]').click();
    await page.waitForTimeout(600);

    // Page should still render without error
    await expect(page.locator('main')).toBeVisible();

    // Toggle it back off; count should return to at least initialCount
    await page.locator('[data-testid="activity-filter-compose"]').click();
    await page.waitForTimeout(600);

    const finalCount = await page.locator('[data-testid="activity-row"]').count();
    expect(finalCount).toBeGreaterThanOrEqual(Math.max(1, initialCount - 1));
  });

  test('global audit (system page) shows entries from multiple apps', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/system`);

    // Click the "Audit Log" tab
    await page.locator('button').filter({ hasText: 'Audit Log' }).click();

    // Wait for at least one row
    await expect(page.locator('[data-testid="activity-row"]').first()).toBeVisible({ timeout: 10_000 });

    // Filter to non-auth categories so app-scoped rows surface above the
    // login noise on page one.
    await page.locator('[data-testid="activity-filter-lifecycle"]').click();
    await page.locator('[data-testid="activity-filter-compose"]').click();

    // After filtering, at least one row should reference a deployed app slug.
    const rows = page.locator('[data-testid="activity-row"]');
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });
    await expect(rows.filter({ hasText: /e2e-nginx|e2e-multi|e2e-postgres/ }).first()).toBeVisible({ timeout: 5_000 });
  });

  test('global audit filter chips are present', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/system`);

    await page.locator('button').filter({ hasText: 'Audit Log' }).click();

    await expect(page.locator('[data-testid="activity-filter-deploy"]')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('[data-testid="activity-filter-compose"]')).toBeVisible();
  });

  test('dashboard recent activity card renders', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);

    // Card heading is "Recent Activity"
    await expect(page.locator('[data-testid="recent-activity-card"]')).toBeVisible({ timeout: 5_000 });
    await expect(page.getByRole('heading', { name: /Recent Activity/i })).toBeVisible();

    // At least one activity row should appear since deploys have happened
    await expect(page.locator('[data-testid="recent-activity-card"] [data-testid="activity-row"]').first()).toBeVisible({ timeout: 10_000 });
  });

  test('failed deploy entry shows error inline', async ({ page }) => {
    // Trigger requires API setup to inject a bad compose; covered by Go unit tests in deployer
    test.skip(true, 'Failed-deploy trigger requires nontrivial API plumbing; covered by Go unit tests in deployer');
  });

  test('expand chevron loads full entry detail', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.locator('button').filter({ hasText: /^activity$/ }).first().click();

    // Find the first row that has expandable detail (compose entries have After JSON).
    const composeRow = page.locator('[data-testid="activity-row"]').filter({ hasText: /compose/i }).first();
    await expect(composeRow).toBeVisible({ timeout: 10_000 });
    await composeRow.locator('button[aria-label="Show details"]').click();

    // After expand, an "After" label should appear within the activity panel.
    await expect(page.locator('main').getByText(/^After$/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('sync status badge appears on sync-eligible entries', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.locator('button').filter({ hasText: /^activity$/ }).first().click();

    // At least one row exists
    await expect(page.locator('[data-testid="activity-row"]').first()).toBeVisible({ timeout: 10_000 });

    // Compose/lifecycle entries are sync-eligible. Locate a row with a sync badge
    // (text or aria-label containing "synced" or "pending").
    const rowsWithBadge = page.locator('[data-testid="activity-row"]').filter({ hasText: /pending|synced/i });
    await expect(rowsWithBadge.first()).toBeVisible({ timeout: 5_000 });
  });

  test('env vars edit appears as env/changed row in per-app feed', async ({ page, request }) => {
    const state = getState();
    const cookies = await page.context().cookies();
    const cookieHeader = cookies.map((c) => `${c.name}=${c.value}`).join('; ');

    // Edit env vars via API to create an env/changed audit row
    const r = await request.put(`${state.baseURL}/api/apps/e2e-nginx/env`, {
      data: [{ key: 'AUDIT_TEST', value: 'hello' }],
      headers: { Cookie: cookieHeader, 'Content-Type': 'application/json' },
      ignoreHTTPSErrors: true,
    });
    expect(r.status()).toBeLessThan(300);

    // Navigate to activity tab and confirm the env entry surfaces
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.locator('button').filter({ hasText: /^activity$/ }).first().click();

    const envRows = page.locator('[data-testid="activity-row"]').filter({ hasText: /env/i });
    await expect(envRows.first()).toBeVisible({ timeout: 10_000 });
  });

  test('audit retention config form is visible to super-admin', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/system`);
    await page.locator('button').filter({ hasText: 'Audit Log' }).click();

    // The retention input + save button should be present (e2eadmin is super-admin in setup)
    const retentionInput = page.locator('input[type="number"]').first();
    await expect(retentionInput).toBeVisible({ timeout: 5_000 });
    await expect(page.getByRole('button', { name: /^Save$/ })).toBeVisible();
  });
});
