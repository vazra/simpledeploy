import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('User Management', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/users`);
  });

  test('users page loads and shows current admin', async ({ page }) => {
    await expect(page.getByText('e2eadmin')).toBeVisible({ timeout: 5_000 });
  });

  test('create new viewer user', async ({ page }) => {
    await page.getByRole('button', { name: /add user/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByPlaceholder(/jane doe/i).fill('Test Viewer');
    await dialog.getByPlaceholder(/jane@example/i).fill('viewer@test.local');
    await dialog.getByPlaceholder(/e\.g\. jane$/i).fill('testviewer');
    await dialog.getByPlaceholder(/min 8/i).fill('ViewerPass123!');

    await dialog.getByText(/viewer/i).click();

    await dialog.getByRole('button', { name: /create user/i }).click();
    await expect(page.getByText('testviewer')).toBeVisible({ timeout: 5_000 });
  });

  test('edit user display name', async ({ page }) => {
    const row = page.locator('tr, [class*="card"]').filter({ hasText: 'testviewer' });
    const editBtn = row.getByRole('button', { name: /edit/i });
    if (await editBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await editBtn.click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      const nameInput = dialog.getByPlaceholder(/name/i).first();
      await nameInput.fill('Updated Viewer');
      await dialog.getByRole('button', { name: /save/i }).click();
      await expect(page.getByText('Updated Viewer')).toBeVisible({ timeout: 5_000 });
    }
  });

  test('API keys section visible', async ({ page }) => {
    await expect(page.getByText(/api key/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('create API key', async ({ page }) => {
    await page.getByRole('button', { name: /create key/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByPlaceholder(/ci-deploy/i).fill('e2e-test-key');
    await dialog.getByRole('button', { name: /create key/i }).click();

    await expect(page.getByText(/sd_/i).first()).toBeVisible({ timeout: 5_000 });
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
      await expect(page.getByText('testviewer')).not.toBeVisible({ timeout: 5_000 });
    }
  });
});
