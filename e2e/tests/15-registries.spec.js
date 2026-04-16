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

    // SlidePanel opens with role="dialog"
    const panel = page.getByRole('dialog');
    await expect(panel).toBeVisible({ timeout: 5_000 });

    // Form fields in order: Name (no placeholder), URL, Username, Password
    const inputs = panel.locator('input');
    await inputs.nth(0).fill('GitHub CR');  // Name
    await panel.getByPlaceholder('registry.example.com').fill('ghcr.io');  // URL
    await inputs.nth(2).fill('testuser');  // Username
    await inputs.nth(3).fill('testtoken');  // Password

    await panel.getByRole('button', { name: /add registry/i }).click();
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
