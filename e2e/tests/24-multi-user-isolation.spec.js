import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest, apiRequestWithKey } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';

test.describe('Multi-User Isolation', () => {
  let viewerUserId = null;

  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);

    // Create viewer user
    const createRes = await apiRequest('POST', '/api/users', {
      username: 'e2e-viewer',
      password: 'ViewerPass123!',
      role: 'viewer',
      display_name: 'E2E Viewer',
      email: 'e2eviewer@test.local',
    });
    expect(createRes.status).toBe(201);
    viewerUserId = createRes.data.id;

    // Grant access to e2e-nginx only
    const accessRes = await apiRequest('POST', `/api/users/${viewerUserId}/access`, {
      app_slug: 'e2e-nginx',
    });
    expect(accessRes.ok).toBeTruthy();
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);

    if (viewerUserId != null) {
      await apiRequest('DELETE', `/api/users/${viewerUserId}/access/e2e-nginx`);
      await apiRequest('DELETE', `/api/users/${viewerUserId}`);
    }
  });

  test.describe('App Isolation (as viewer)', () => {
    test.beforeAll(async () => {
      await apiLogin('e2e-viewer', 'ViewerPass123!');
    });

    test('GET /api/apps returns only accessible apps', async () => {
      const res = await apiRequest('GET', '/api/apps');
      expect(res.status).toBe(200);
      const slugs = res.data.map((a) => a.slug);
      expect(slugs).toContain('e2e-nginx');
      expect(slugs).not.toContain('e2e-multi');
      expect(slugs).not.toContain('e2e-postgres');
    });

    test('GET /api/apps/e2e-nginx returns 200', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx');
      expect(res.status).toBe(200);
    });

    test('GET /api/apps/e2e-multi returns 404', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-multi');
      expect(res.status).toBe(404);
    });

    test('GET /api/apps/e2e-postgres returns 404', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-postgres');
      expect(res.status).toBe(404);
    });
  });

  test.describe('Role Restrictions (as viewer)', () => {
    test.beforeAll(async () => {
      await apiLogin('e2e-viewer', 'ViewerPass123!');
    });

    test('POST /api/users returns 403', async () => {
      const res = await apiRequest('POST', '/api/users', {
        username: 'should-fail',
        password: 'ShouldFail123!',
        role: 'viewer',
      });
      expect(res.status).toBe(403);
    });

    test('DELETE /api/apps/e2e-nginx returns 403', async () => {
      const res = await apiRequest('DELETE', '/api/apps/e2e-nginx');
      expect(res.status).toBe(403);
    });
  });

  test.describe('API Key Auth', () => {
    let keyId = null;
    let plainTextKey = null;

    test.beforeAll(async () => {
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    });

    test('create API key', async () => {
      const res = await apiRequest('POST', '/api/apikeys', { name: 'e2e-key-test' });
      expect(res.status).toBe(201);
      expect(res.data.key).toMatch(/^sd_/);
      keyId = res.data.id;
      plainTextKey = res.data.key;
    });

    test('API key can list apps', async () => {
      expect(plainTextKey).toBeTruthy();
      const res = await apiRequestWithKey('GET', '/api/apps', null, plainTextKey);
      expect(res.status).toBe(200);
      expect(Array.isArray(res.data)).toBe(true);
    });

    test('revoked key returns 401', async () => {
      expect(keyId).toBeTruthy();
      const revokeRes = await apiRequest('DELETE', `/api/apikeys/${keyId}`);
      expect(revokeRes.ok).toBeTruthy();

      const res = await apiRequestWithKey('GET', '/api/apps', null, plainTextKey);
      expect(res.status).toBe(401);
    });
  });
});
