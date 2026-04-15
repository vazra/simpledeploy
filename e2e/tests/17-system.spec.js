import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('System Administration', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/system`);
  });

  test('system overview loads', async ({ page }) => {
    await expect(page.getByText(/simpledeploy/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/uptime/i).first()).toBeVisible();
  });

  test('shows database info', async ({ page }) => {
    await expect(page.getByText(/database|sqlite/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('shows system resources', async ({ page }) => {
    await expect(page.getByText(/cpu|cores/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/ram|memory/i).first()).toBeVisible();
  });

  test('maintenance tab - vacuum database', async ({ page }) => {
    await page.getByRole('button', { name: /maintenance/i }).click();
    const vacuumBtn = page.getByRole('button', { name: /vacuum/i });
    await expect(vacuumBtn).toBeVisible({ timeout: 5_000 });
    await vacuumBtn.click();
    await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 10_000 });
  });

  test('maintenance tab - prune metrics', async ({ page }) => {
    await page.getByRole('button', { name: /maintenance/i }).click();
    const tierSelect = page.locator('select').first();
    await expect(tierSelect).toBeVisible({ timeout: 5_000 });
  });

  test('maintenance tab - download database backup', async ({ page }) => {
    await page.getByRole('button', { name: /maintenance/i }).click();
    const downloadBtn = page.getByRole('button', { name: /download/i });
    await expect(downloadBtn).toBeVisible({ timeout: 5_000 });
  });

  test('audit log tab loads', async ({ page }) => {
    await page.getByRole('button', { name: /audit/i }).click();
    await expect(page.getByText(/time|event/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('logs tab loads', async ({ page }) => {
    await page.getByRole('button', { name: /logs/i }).click();
    await expect(page.getByText(/auto-scroll|refresh/i).first()).toBeVisible({ timeout: 5_000 });
  });
});
