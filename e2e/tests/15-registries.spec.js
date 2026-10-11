import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

// UI add/delete only; 29b covers a real authenticated pull.
test.describe('Registries', () => {
  test('add a registry, then delete it', async ({ page }) => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/registries`);

    await page.getByRole('button', { name: 'Add Registry' }).click();
    const panel = page.getByRole('dialog');
    const field = (label) => panel.locator(`div:has(> label:text-is("${label}")) > input`);
    await field('Name').fill('GitHub CR');
    await field('URL').fill('ghcr.io');
    await field('Username').fill('testuser');
    await field('Password').fill('testtoken');
    await panel.getByRole('button', { name: 'Add Registry' }).click();

    const row = page.locator('main table tr').filter({ hasText: 'ghcr.io' });
    await expect(row).toContainText('GitHub CR');
    await expect(row).toContainText('testuser');
    const list = await apiRequest('GET', '/api/registries');
    expect(list.status).toBe(200);
    const reg = list.data.find((r) => r.url === 'ghcr.io');
    expect(reg).toMatchObject({ name: 'GitHub CR', username: 'testuser' });
    // Credentials are write-only.
    expect(JSON.stringify(list.data)).not.toContain('testtoken');

    await row.getByRole('button', { name: 'Delete' }).click();
    await expect(row).toHaveCount(0);
    const after = await apiRequest('GET', '/api/registries');
    expect(after.data.find((r) => r.url === 'ghcr.io')).toBeUndefined();
  });
});
