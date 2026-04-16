import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
  });

  test('shows all 3 deployed app cards', async ({ page }) => {
    await expect(page.getByText('e2e-nginx')).toBeVisible();
    await expect(page.getByText('e2e-multi')).toBeVisible();
    await expect(page.getByText('e2e-postgres')).toBeVisible();
  });

  test('app cards show running status', async ({ page }) => {
    // Badge text "running" appears on app cards
    await expect(page.locator('main').getByText('running').first()).toBeVisible();
  });

  test('system metrics section renders', async ({ page }) => {
    await expect(page.getByText(/cpu/i).first()).toBeVisible();
    await expect(page.getByText(/memory/i).first()).toBeVisible();
  });

  test('search filters apps', async ({ page }) => {
    await page.getByPlaceholder(/search/i).fill('nginx');
    await expect(page.getByText('e2e-nginx')).toBeVisible();
    await expect(page.getByText('e2e-multi')).not.toBeVisible();
    await expect(page.getByText('e2e-postgres')).not.toBeVisible();
  });

  test('clicking app card navigates to app detail', async ({ page }) => {
    await page.getByText('e2e-nginx').click();
    await expect(page).toHaveURL(/apps\/e2e-nginx/);
  });
});
