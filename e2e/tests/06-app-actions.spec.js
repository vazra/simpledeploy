import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('App Actions', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('stop app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /stop/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const confirmBtn = dialog.getByRole('button', { name: /stop|confirm|yes/i });
      if (await confirmBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await confirmBtn.click();
      }
    }
    await expect(page.getByText(/stopped/i).first()).toBeVisible({ timeout: 30_000 });
  });

  test('start app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /start/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await dialog.getByRole('button', { name: /close/i }).click();
    }
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 30_000 });
  });

  test('restart app', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /restart/i }).click();
    const dialog = page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const confirmBtn = dialog.getByRole('button', { name: /restart|confirm/i });
      if (await confirmBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await confirmBtn.click();
      }
    }
    const closeBtn = page.getByRole('button', { name: /close/i });
    await expect(closeBtn).toBeVisible({ timeout: 60_000 });
    await closeBtn.click();
    await expect(page.getByText(/running/i).first()).toBeVisible({ timeout: 15_000 });
  });

  test('pull and update', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: /pull/i }).click();
    const closeBtn = page.getByRole('button', { name: /close/i });
    await expect(closeBtn).toBeVisible({ timeout: 120_000 });
    await closeBtn.click();
  });

  test('scale service', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-multi`);
    const scaleBtn = page.getByRole('button', { name: /scale/i });
    if (await scaleBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await scaleBtn.click();
    } else {
      const moreBtn = page.getByRole('button', { name: /more/i });
      if (await moreBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await moreBtn.click();
        await page.getByText(/scale/i).click();
      }
    }
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    const inputs = dialog.locator('input[type="number"]');
    const count = await inputs.count();
    if (count > 0) {
      for (let i = 0; i < count; i++) {
        const label = await inputs.nth(i).evaluate(el => {
          const row = el.closest('div');
          return row?.textContent || '';
        });
        if (label.includes('cache')) {
          await inputs.nth(i).fill('2');
          break;
        }
      }
    }
    const applyBtn = dialog.getByRole('button', { name: /apply|scale|confirm/i });
    await applyBtn.click();
    const closeBtn = page.getByRole('button', { name: /close/i });
    await expect(closeBtn).toBeVisible({ timeout: 60_000 });
    await closeBtn.click();
  });
});
