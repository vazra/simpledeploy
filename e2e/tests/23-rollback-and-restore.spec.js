import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';

test.describe('Rollback and Restore', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.describe('Deploy Versions', () => {
    test('GET versions returns array with at least 1 entry', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
      expect(res.ok).toBeTruthy();
      expect(Array.isArray(res.data)).toBe(true);
      expect(res.data.length).toBeGreaterThanOrEqual(1);
    });

    test('each version entry has id and created_at fields', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
      expect(res.ok).toBeTruthy();
      expect(res.data.length).toBeGreaterThanOrEqual(1);
      for (const v of res.data) {
        expect(typeof v.id).toBe('number');
        expect(v.created_at).toBeTruthy();
      }
    });
  });

  test.describe('Rollback', () => {
    test('rollback to oldest version succeeds and app reaches running', async () => {
      const versionsRes = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
      expect(versionsRes.ok).toBeTruthy();
      expect(versionsRes.data.length).toBeGreaterThanOrEqual(1);

      // versions are newest-first; oldest is last
      const oldest = versionsRes.data[versionsRes.data.length - 1];
      expect(typeof oldest.id).toBe('number');

      const rollbackRes = await apiRequest('POST', '/api/apps/e2e-nginx/rollback', {
        version_id: oldest.id,
      });
      expect(rollbackRes.status).toBe(200);

      await waitForAppStatus('e2e-nginx', 'running', 90_000);
    });

    test('GET events shows at least one entry after rollback', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx/events');
      expect(res.ok).toBeTruthy();
      expect(Array.isArray(res.data)).toBe(true);
      expect(res.data.length).toBeGreaterThanOrEqual(1);
    });
  });

  test.describe('Rollback Validation', () => {
    test('rollback with version_id 0 returns 400', async () => {
      const res = await apiRequest('POST', '/api/apps/e2e-nginx/rollback', {
        version_id: 0,
      });
      expect(res.status).toBe(400);
    });

    test('rollback with nonexistent version_id returns error', async () => {
      const res = await apiRequest('POST', '/api/apps/e2e-nginx/rollback', {
        version_id: 999999,
      });
      expect(res.ok).toBe(false);
    });
  });

  test.describe('Backup Restore (conditional)', () => {
    test('restore succeeds if a successful backup run exists, otherwise skips', async () => {
      const runsRes = await apiRequest('GET', '/api/apps/e2e-postgres/backups/runs');
      expect(runsRes.ok).toBeTruthy();
      expect(Array.isArray(runsRes.data)).toBe(true);

      const successRun = runsRes.data.find((r) => r.status === 'success');
      if (!successRun) {
        // No successful backup run available - skip gracefully
        return;
      }

      const restoreRes = await apiRequest('POST', `/api/backups/restore/${successRun.id}`);
      expect(restoreRes.status).toBe(202);
    });
  });
});
