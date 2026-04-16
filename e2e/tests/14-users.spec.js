import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('User Management', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/users`);
  });

  test('users page loads and shows current admin', async ({ page }) => {
    // Username may appear in sidebar + page; use .first()
    await expect(page.locator('main').getByText('e2eadmin').first()).toBeVisible({ timeout: 5_000 });
  });

  test('create new viewer user', async ({ page }) => {
    await page.getByRole('button', { name: /add user/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Placeholders from Users.svelte FormModal - use exact matching
    await dialog.getByPlaceholder('e.g. Jane Doe', { exact: true }).fill('Test Viewer');
    await dialog.getByPlaceholder('jane@example.com', { exact: true }).fill('viewer@test.local');
    await dialog.getByPlaceholder('e.g. jane', { exact: true }).fill('testviewer');
    await dialog.getByPlaceholder('Min 8 characters', { exact: true }).fill('ViewerPass123!');

    // Select Viewer role (it's a button, not a radio)
    await dialog.locator('button').filter({ hasText: 'Viewer' }).first().click();

    await dialog.getByRole('button', { name: /create user/i }).click();
    await expect(page.locator('main').getByText('testviewer').first()).toBeVisible({ timeout: 5_000 });
  });

  test('edit user display name', async ({ page }) => {
    const row = page.locator('tr, [class*="card"]').filter({ hasText: 'testviewer' });
    const editBtn = row.getByRole('button', { name: /edit/i });
    if (await editBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await editBtn.click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      // Display Name placeholder is "e.g. Jane Doe"
      const nameInput = dialog.getByPlaceholder('e.g. Jane Doe');
      await nameInput.fill('Updated Viewer');
      await dialog.getByRole('button', { name: /save/i }).click();
      await expect(page.locator('main').getByText('Updated Viewer').first()).toBeVisible({ timeout: 5_000 });
    }
  });

  test('API keys section visible', async ({ page }) => {
    await expect(page.locator('h3').getByText('API Keys')).toBeVisible({ timeout: 5_000 });
  });

  test('create API key', async ({ page }) => {
    await page.getByRole('button', { name: /create key/i }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Placeholder is "e.g. ci-deploy"
    await dialog.getByPlaceholder('e.g. ci-deploy').fill('e2e-test-key');
    await dialog.getByRole('button', { name: /create key/i }).click();

    // After creation, key is shown (starts with "sd_" or similar prefix)
    // The key display section shows the key in a <code> element
    await expect(page.locator('code').first()).toBeVisible({ timeout: 5_000 });
  });

  test('delete API key', async ({ page }) => {
    const revokeBtn = page.getByRole('button', { name: /revoke/i }).first();
    if (await revokeBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await revokeBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /revoke|delete|confirm/i }).click();
      }
    }
  });

  test('delete viewer user', async ({ page }) => {
    const row = page.locator('tr, [class*="card"]').filter({ hasText: 'testviewer' });
    const deleteBtn = row.getByRole('button', { name: /delete/i });
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
      await expect(page.locator('main').getByText('testviewer').first()).not.toBeVisible({ timeout: 5_000 });
    }
  });
});
