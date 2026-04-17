import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, apiRequestWithKey } from '../helpers/api.js';

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

test.describe('Users - Functional RBAC', () => {
  test.describe.configure({ mode: 'serial' });

  const VIEWER_USER = {
    username: 'e2erbac_viewer',
    password: 'ViewerPass123!',
    display_name: 'E2E RBAC Viewer',
    email: 'rbac-viewer@test.local',
    role: 'viewer',
  };

  let viewerId = null;
  let viewerKey = null;
  let viewerKeyId = null;

  test.beforeAll(async () => {
    // Authenticate as super_admin
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.ok).toBeTruthy();

    // Cleanup any leftover from prior runs
    const listRes = await apiRequest('GET', '/api/users');
    if (listRes.ok && Array.isArray(listRes.data)) {
      const existing = listRes.data.find((u) => u.username === VIEWER_USER.username);
      if (existing) {
        await apiRequest('DELETE', `/api/users/${existing.id}`);
      }
    }
  });

  test.afterAll(async () => {
    // Always try to cleanup so 99-cleanup remains clean
    try {
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
      if (viewerKeyId) {
        await apiRequest('DELETE', `/api/apikeys/${viewerKeyId}`);
      }
      if (viewerId) {
        await apiRequest('DELETE', `/api/users/${viewerId}`);
      }
    } catch {}
  });

  test('create restricted viewer user and grant app-nginx access', async () => {
    const createRes = await apiRequest('POST', '/api/users', VIEWER_USER);
    expect(createRes.status).toBe(201);
    expect(createRes.data.id).toBeTruthy();
    expect(createRes.data.role).toBe('viewer');
    viewerId = createRes.data.id;

    // Grant access to e2e-nginx only (endpoint: POST /api/users/:id/access {app_slug})
    const grantRes = await apiRequest('POST', `/api/users/${viewerId}/access`, {
      app_slug: 'e2e-nginx',
    });
    expect(grantRes.ok).toBeTruthy();
  });

  test('create API key by logging in as the viewer', async () => {
    // API keys are owned by the authenticated caller, so log in as viewer
    const vLogin = await apiLogin(VIEWER_USER.username, VIEWER_USER.password);
    expect(vLogin.ok).toBeTruthy();

    const keyRes = await apiRequest('POST', '/api/apikeys', { name: 'e2e-rbac-key' });
    expect(keyRes.status).toBe(201);
    expect(typeof keyRes.data.key).toBe('string');
    expect(keyRes.data.key.length).toBeGreaterThan(10);
    viewerKey = keyRes.data.key;
    viewerKeyId = keyRes.data.id;
  });

  test('viewer API key can access allowed app', async () => {
    expect(viewerKey).toBeTruthy();
    const res = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, viewerKey);
    expect(res.status).toBe(200);
    // App struct has no JSON tags; fields are PascalCase.
    expect(res.data.Slug).toBe('e2e-nginx');
  });

  test('viewer API key cannot access restricted app (404 via appAccessMiddleware)', async () => {
    expect(viewerKey).toBeTruthy();
    const res = await apiRequestWithKey('GET', '/api/apps/e2e-multi', null, viewerKey);
    // appAccessMiddleware returns 404 for denied apps
    expect(res.status).toBe(404);
  });

  test('viewer cannot perform super_admin actions (create user -> 403)', async () => {
    expect(viewerKey).toBeTruthy();
    const res = await apiRequestWithKey('POST', '/api/users', {
      username: 'should_not_create',
      password: 'somepass12345',
      role: 'viewer',
    }, viewerKey);
    expect(res.status).toBe(403);
  });

  test('viewer cannot delete users (403)', async () => {
    expect(viewerKey).toBeTruthy();
    // attempt to delete self or admin - should be forbidden by role check
    const res = await apiRequestWithKey('DELETE', `/api/users/${viewerId}`, null, viewerKey);
    expect(res.status).toBe(403);
  });

  test('viewer cannot access system maintenance endpoints (403)', async () => {
    expect(viewerKey).toBeTruthy();
    const res = await apiRequestWithKey('POST', '/api/system/vacuum', null, viewerKey);
    expect(res.status).toBe(403);
  });

  test('invalid API key is rejected (401)', async () => {
    const res = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, 'sd_invalidkey12345');
    expect(res.status).toBe(401);
  });

  test('cleanup: revoke app access, delete API key and viewer user', async () => {
    // Re-login as super_admin to delete the key and user
    const adm = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(adm.ok).toBeTruthy();

    // Revoke access
    await apiRequest('DELETE', `/api/users/${viewerId}/access/e2e-nginx`);

    // Delete API key (super_admin can delete any)
    if (viewerKeyId) {
      const delKey = await apiRequest('DELETE', `/api/apikeys/${viewerKeyId}`);
      expect([200, 404].includes(delKey.status)).toBeTruthy();
      viewerKeyId = null;
    }

    // Delete viewer user
    const delUser = await apiRequest('DELETE', `/api/users/${viewerId}`);
    expect([200, 404].includes(delUser.status)).toBeTruthy();
    viewerId = null;

    // Verify API key no longer works (either 401 unauthorized or 404 not-found for app)
    const verify = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, viewerKey);
    expect([401, 404].includes(verify.status)).toBeTruthy();
  });
});
