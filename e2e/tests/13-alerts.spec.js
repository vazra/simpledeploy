import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Alerts & Webhooks', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/alerts`);
  });

  test('alerts page loads', async ({ page }) => {
    await expect(page.getByText(/webhook/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/alert rule/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('create webhook', async ({ page }) => {
    await page.getByRole('button', { name: /add webhook/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByPlaceholder(/my webhook/i).fill('E2E Test Hook');
    const typeSelect = dialog.locator('select').first();
    await typeSelect.selectOption('custom');
    await dialog.getByPlaceholder(/https:\/\//i).fill('https://httpbin.org/post');

    await dialog.getByRole('button', { name: /create/i }).click();
    await expect(page.getByText('E2E Test Hook')).toBeVisible({ timeout: 5_000 });
  });

  test('test webhook', async ({ page }) => {
    const testBtn = page.getByRole('button', { name: /test/i }).first();
    if (await testBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await testBtn.click();
      await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 10_000 });
    }
  });

  test('create alert rule', async ({ page }) => {
    await page.getByRole('button', { name: /add rule/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    const nameInput = dialog.getByPlaceholder(/name|rule/i);
    if (await nameInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await nameInput.fill('E2E CPU Alert');
    }

    const metricSelect = dialog.locator('select').filter({ hasText: /cpu|metric/i }).first();
    if (await metricSelect.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await metricSelect.selectOption({ index: 0 });
    }

    const thresholdInput = dialog.locator('input[type="number"]').first();
    if (await thresholdInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await thresholdInput.fill('90');
    }

    const webhookSelect = dialog.locator('select').last();
    if (await webhookSelect.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const options = await webhookSelect.locator('option').allTextContents();
      if (options.length > 1) {
        await webhookSelect.selectOption({ index: 1 });
      }
    }

    await dialog.getByRole('button', { name: /create|save/i }).click();
    await expect(page.getByText(/cpu/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('view alert history', async ({ page }) => {
    await expect(page.getByText(/history/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('delete alert rule', async ({ page }) => {
    const deleteBtn = page.locator('button').filter({ hasText: /delete/i }).last();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });

  test('delete webhook', async ({ page }) => {
    const deleteBtn = page.locator('button').filter({ hasText: /delete/i }).first();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });
});
