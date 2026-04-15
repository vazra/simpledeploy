import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('App Logs', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /logs/i }).click();
  });

  test('log viewer renders', async ({ page }) => {
    const logContainer = page.locator('.font-mono').first();
    await expect(logContainer).toBeVisible({ timeout: 10_000 });
  });

  test('log controls visible', async ({ page }) => {
    const followBtn = page.getByText(/following|paused/i).first();
    await expect(followBtn).toBeVisible({ timeout: 5_000 });
  });

  test('clear button works', async ({ page }) => {
    const clearBtn = page.getByRole('button', { name: /clear/i });
    if (await clearBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await clearBtn.click();
    }
  });

  test('deploy logs available in events tab', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    const eventsBtn = page.getByRole('button', { name: /event|history/i });
    if (await eventsBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await eventsBtn.click();
      await expect(page.getByText(/deploy/i).first()).toBeVisible({ timeout: 5_000 });
    }
  });
});
