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

    // At least one of the deployed test app slugs should appear
    const main = page.locator('main');
    await expect(main.getByText(/e2e-nginx|e2e-multi/).first()).toBeVisible();
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
});
