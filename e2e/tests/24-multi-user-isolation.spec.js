import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { deleteUserByName } from '../helpers/users.js';

// Viewer read isolation, API keys and super_admin guards live in 14-users.
// This spec checks the viewer role through a cookie session: it can never
// mutate, not even an app it was granted.
test.describe('Multi-User Isolation', () => {
  const VIEWER = 'e2e-viewer';

  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    await deleteUserByName(VIEWER);
    const createRes = await apiRequest('POST', '/api/users', {
      username: VIEWER,
      password: 'ViewerPass123!',
      role: 'viewer',
      display_name: 'E2E Viewer',
      email: 'e2eviewer@test.local',
    });
    expect(createRes.status).toBe(201);
    const grant = await apiRequest('POST', `/api/users/${createRes.data.id}/access`, { app_slug: 'e2e-nginx' });
    expect(grant.status).toBe(200);
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    await deleteUserByName(VIEWER);
  });

  test('viewer session gets 403 on mutating routes, granted app or not', async () => {
    const envBefore = (await apiRequest('GET', '/api/apps/e2e-nginx/env')).data;
    expect((await apiLogin(VIEWER, 'ViewerPass123!')).status).toBe(200);
    expect((await apiRequest('GET', '/api/apps/e2e-nginx')).status).toBe(200);

    // Role checks run before the access check, so an ungranted app answers
    // 403, not 404.
    expect((await apiRequest('DELETE', '/api/apps/e2e-multi')).status).toBe(403);
    expect((await apiRequest('POST', '/api/apps/e2e-nginx/restart')).status).toBe(403);
    expect((await apiRequest('PUT', '/api/apps/e2e-nginx/env', [{ key: 'X', value: '1' }])).status).toBe(403);
    expect((await apiRequest('PUT', '/api/apps/e2e-nginx/access', { allow: '10.0.0.1' })).status).toBe(403);

    // Nothing changed.
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    expect((await apiRequest('GET', '/api/apps/e2e-multi')).status).toBe(200);
    expect((await apiRequest('GET', '/api/apps/e2e-nginx/env')).data).toEqual(envBefore);
  });
});
