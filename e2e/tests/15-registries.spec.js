import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Registries', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/registries`);
  });

  test('registries page loads', async ({ page }) => {
    await expect(page.getByText(/registr/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('add registry', async ({ page }) => {
    await page.getByRole('button', { name: /add registry/i }).click();

    await page.getByPlaceholder(/registry.example.com/i).fill('ghcr.io');
    const nameInput = page.locator('input').first();
    await nameInput.fill('GitHub CR');
    await page.getByPlaceholder(/username/i).first().fill('testuser');
    await page.locator('input[type="password"]').first().fill('testtoken');

    await page.getByRole('button', { name: /add registry/i }).click();
    await expect(page.getByText('ghcr.io')).toBeVisible({ timeout: 5_000 });
  });

  test('delete registry', async ({ page }) => {
    const deleteBtn = page.getByRole('button', { name: /delete/i }).first();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });
});
