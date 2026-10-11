import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, apiRequestWithKey } from '../helpers/api.js';
import { deleteUserByName, findUser, latestDeployEventId, waitForDeployEvent } from '../helpers/users.js';

test.describe('User Management', () => {
  test.describe.configure({ mode: 'serial' });

  const UI_USER = 'e2e-ui-user';
  const UI_KEY = 'e2e-test-key';

  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    await deleteUserByName(UI_USER);
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    await deleteUserByName(UI_USER);
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/users`);
  });

  test('create user with the Manage role', async ({ page }) => {
    await page.getByRole('button', { name: 'Add User' }).first().click();
    const dialog = page.getByRole('dialog');
    await dialog.getByPlaceholder('e.g. Jane Doe', { exact: true }).fill('Test User');
    await dialog.getByPlaceholder('jane@example.com', { exact: true }).fill('ui-user@test.local');
    await dialog.getByPlaceholder('e.g. jane', { exact: true }).fill(UI_USER);
    await dialog.getByPlaceholder('Min 8 characters', { exact: true }).fill('UiUserPass123!');
    // Viewer is the default, so pick Manage to prove the choice is sent.
    await dialog.getByRole('button', { name: /^Manage/ }).click();
    await dialog.getByRole('button', { name: 'Create User' }).click();

    await expect(dialog).toBeHidden();
    await expect(page.locator('main table tr').filter({ hasText: `@${UI_USER}` })).toContainText('manage');
    const u = await findUser(UI_USER);
    expect(u).toMatchObject({ role: 'manage', display_name: 'Test User', email: 'ui-user@test.local' });
  });

  test('edit display name and role', async ({ page }) => {
    const row = page.locator('main table tr').filter({ hasText: `@${UI_USER}` });
    await row.getByRole('button', { name: 'Edit' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByPlaceholder('e.g. Jane Doe').fill('Updated User');
    await dialog.getByRole('button', { name: /^Viewer/ }).click();
    await dialog.getByRole('button', { name: 'Save Changes' }).click();

    await expect(dialog).toBeHidden();
    await expect(row).toContainText('Updated User');
    await expect(row).toContainText('viewer');
    const u = await findUser(UI_USER);
    expect(u).toMatchObject({ role: 'viewer', display_name: 'Updated User', email: 'ui-user@test.local' });
  });

  test('create API key, use it, then revoke it', async ({ page }) => {
    await page.getByRole('button', { name: 'Create Key' }).first().click();
    const dialog = page.getByRole('dialog');
    await dialog.getByPlaceholder('e.g. ci-deploy').fill(UI_KEY);
    await dialog.getByRole('button', { name: 'Create Key' }).click();
    await expect(dialog).toBeHidden();

    const code = page.locator('main code').filter({ hasText: /^sd_/ });
    await expect(code).toHaveText(/^sd_[0-9a-f]{64}$/);
    const key = (await code.textContent()).trim();
    const keyRow = page.locator('main table tr').filter({ hasText: UI_KEY });
    await expect(keyRow).toBeVisible();

    // The shown key is the real one: it authenticates as the admin.
    const me = await apiRequestWithKey('GET', '/api/me', null, key);
    expect(me.status).toBe(200);
    expect(me.data.username).toBe(TEST_ADMIN.username);

    await keyRow.getByRole('button', { name: 'Revoke' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Confirm' }).click();
    await expect(keyRow).toHaveCount(0);
    const keys = await apiRequest('GET', '/api/apikeys');
    expect(keys.status).toBe(200);
    expect(keys.data.map((k) => k.name)).not.toContain(UI_KEY);
    expect((await apiRequestWithKey('GET', '/api/me', null, key)).status).toBe(401);
  });

  test('delete user', async ({ page }) => {
    const row = page.locator('main table tr').filter({ hasText: `@${UI_USER}` });
    await row.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Confirm' }).click();
    await expect(row).toHaveCount(0);
    expect(await findUser(UI_USER)).toBeUndefined();
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
  const MANAGE_USER = {
    username: 'e2erbac_manage',
    password: 'ManagePass123!',
    display_name: 'E2E RBAC Manage',
    email: 'rbac-manage@test.local',
    role: 'manage',
  };

  let viewerId = null;
  let viewerKey = null;

  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    await deleteUserByName(VIEWER_USER.username);
    await deleteUserByName(MANAGE_USER.username);
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    await deleteUserByName(VIEWER_USER.username);
    await deleteUserByName(MANAGE_USER.username);
  });

  test('create restricted viewer user and grant app-nginx access', async () => {
    const createRes = await apiRequest('POST', '/api/users', VIEWER_USER);
    expect(createRes.status).toBe(201);
    expect(createRes.data.role).toBe('viewer');
    viewerId = createRes.data.id;

    const grantRes = await apiRequest('POST', `/api/users/${viewerId}/access`, {
      app_slug: 'e2e-nginx',
    });
    expect(grantRes.status).toBe(200);
  });

  test('create API key by logging in as the viewer', async () => {
    // API keys are owned by the authenticated caller, so log in as viewer
    const vLogin = await apiLogin(VIEWER_USER.username, VIEWER_USER.password);
    expect(vLogin.status).toBe(200);

    const keyRes = await apiRequest('POST', '/api/apikeys', { name: 'e2e-rbac-key' });
    expect(keyRes.status).toBe(201);
    expect(keyRes.data.key).toMatch(/^sd_[0-9a-f]{64}$/);
    viewerKey = keyRes.data.key;
  });

  test('viewer API key can access allowed app', async () => {
    const res = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, viewerKey);
    expect(res.status).toBe(200);
    // App struct has no JSON tags; fields are PascalCase.
    expect(res.data.Slug).toBe('e2e-nginx');
  });

  test('viewer API key cannot access restricted app (404 via appAccessMiddleware)', async () => {
    const res = await apiRequestWithKey('GET', '/api/apps/e2e-multi', null, viewerKey);
    expect(res.status).toBe(404);
  });

  test('viewer cannot create or delete users or run maintenance (403)', async () => {
    const create = await apiRequestWithKey('POST', '/api/users', {
      username: 'should_not_create',
      password: 'somepass12345',
      role: 'viewer',
    }, viewerKey);
    expect(create.status).toBe(403);
    const del = await apiRequestWithKey('DELETE', `/api/users/${viewerId}`, null, viewerKey);
    expect(del.status).toBe(403);
    const vacuum = await apiRequestWithKey('POST', '/api/system/vacuum', null, viewerKey);
    expect(vacuum.status).toBe(403);
  });

  test('invalid API key is rejected (401)', async () => {
    const res = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, 'sd_invalidkey12345');
    expect(res.status).toBe(401);
  });

  test('dashboard list filters apps for restricted viewer', async () => {
    const res = await apiRequestWithKey('GET', '/api/apps', null, viewerKey);
    expect(res.status).toBe(200);
    const slugs = res.data.map((a) => a.Slug);
    expect(slugs).toContain('e2e-nginx');
    expect(slugs).not.toContain('e2e-multi');
    expect(slugs).not.toContain('e2e-postgres');
  });

  test('GET /api/users/:id/access lists granted slugs (super_admin only)', async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    const res = await apiRequest('GET', `/api/users/${viewerId}/access`);
    expect(res.status).toBe(200);
    expect(res.data).toEqual(['e2e-nginx']);

    const forbidden = await apiRequestWithKey('GET', `/api/users/${viewerId}/access`, null, viewerKey);
    expect(forbidden.status).toBe(403);
  });

  test('manage user: granted app can mutate; ungranted app and platform endpoints denied', async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    const created = await apiRequest('POST', '/api/users', MANAGE_USER);
    expect(created.status).toBe(201);
    expect(created.data.role).toBe('manage');
    const grant = await apiRequest('POST', `/api/users/${created.data.id}/access`, { app_slug: 'e2e-nginx' });
    expect(grant.status).toBe(200);

    expect((await apiLogin(MANAGE_USER.username, MANAGE_USER.password)).status).toBe(200);
    const keyRes = await apiRequest('POST', '/api/apikeys', { name: 'e2e-rbac-manage' });
    expect(keyRes.status).toBe(201);
    const mKey = keyRes.data.key;
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);

    const readOk = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, mKey);
    expect(readOk.status).toBe(200);

    // Restart runs in the background; wait for it so later specs get a settled app.
    const before = await latestDeployEventId('e2e-nginx');
    const restartOk = await apiRequestWithKey('POST', '/api/apps/e2e-nginx/restart', null, mKey);
    expect(restartOk.status).toBe(202);
    expect((await waitForDeployEvent('e2e-nginx', before)).action).toBe('restart');

    // Cannot read or mutate an ungranted app (404 from access middleware).
    const otherRead = await apiRequestWithKey('GET', '/api/apps/e2e-multi', null, mKey);
    expect(otherRead.status).toBe(404);
    const otherRestart = await apiRequestWithKey('POST', '/api/apps/e2e-multi/restart', null, mKey);
    expect(otherRestart.status).toBe(404);

    // super_admin only: deploy, delete app, users, system, registries.
    const deployRes = await apiRequestWithKey('POST', '/api/apps/deploy',
      { name: 'should-not-exist', compose: 'services: {}\n' }, mKey);
    expect(deployRes.status).toBe(403);
    const delApp = await apiRequestWithKey('DELETE', '/api/apps/e2e-nginx', null, mKey);
    expect(delApp.status).toBe(403);
    const listUsers = await apiRequestWithKey('GET', '/api/users', null, mKey);
    expect(listUsers.status).toBe(403);
    const sysVac = await apiRequestWithKey('POST', '/api/system/vacuum', null, mKey);
    expect(sysVac.status).toBe(403);
    const regs = await apiRequestWithKey('GET', '/api/registries', null, mKey);
    expect(regs.status).toBe(403);
  });

  test('UI: grant additional app access via Users edit modal', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/users`);

    const row = page.locator('main table tr').filter({ hasText: `@${VIEWER_USER.username}` });
    await row.getByRole('button', { name: 'Edit' }).click();
    const dialog = page.getByRole('dialog');
    const multiCheckbox = dialog.locator('label').filter({ hasText: 'e2e-multi' }).locator('input[type="checkbox"]');
    await expect(multiCheckbox).not.toBeChecked();

    const grantP = page.waitForResponse((r) => r.url().endsWith(`/api/users/${viewerId}/access`) && r.request().method() === 'POST');
    await multiCheckbox.check();
    expect((await grantP).status()).toBe(200);
    const accessRes = await apiRequest('GET', `/api/users/${viewerId}/access`);
    expect(accessRes.data.sort()).toEqual(['e2e-multi', 'e2e-nginx']);

    const revokeP = page.waitForResponse((r) => r.url().endsWith(`/api/users/${viewerId}/access/e2e-multi`) && r.request().method() === 'DELETE');
    await multiCheckbox.uncheck();
    expect((await revokeP).status()).toBe(200);
    const after = await apiRequest('GET', `/api/users/${viewerId}/access`);
    expect(after.data).toEqual(['e2e-nginx']);
  });

  test('deleting the viewer revokes their API key', async () => {
    const delUser = await apiRequest('DELETE', `/api/users/${viewerId}`);
    expect(delUser.status).toBe(200);
    viewerId = null;
    const verify = await apiRequestWithKey('GET', '/api/apps/e2e-nginx', null, viewerKey);
    expect(verify.status).toBe(401);
  });
});
