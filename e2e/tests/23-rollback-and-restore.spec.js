import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { findServiceContainer, containerImage } from '../helpers/docker.js';
import { waitForProxyStatus } from '../helpers/proxy.js';
import { waitForAppSettled } from '../helpers/users.js';

// Restore from backup is covered with data checks by 12-backups.
test.describe('Rollback', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test('rollback with version_id 0 returns 400', async () => {
    const res = await apiRequest('POST', '/api/apps/e2e-nginx/rollback', { version_id: 0 });
    expect(res.status).toBe(400);
  });

  test('rollback swaps the running image and roll-forward restores it', async () => {
    // A rollback while a deploy is in flight is skipped, so start settled.
    await waitForAppSettled('e2e-nginx');
    const versions = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
    expect(versions.status).toBe(200);
    // Newest first; 11-app-versions deployed nginx:1.27-alpine over nginx:alpine.
    const latest = versions.data[0];
    expect(latest.content).toContain('nginx:1.27-alpine');
    const older = versions.data.find((v) => v.content.includes('nginx:alpine'));
    expect(older, 'an nginx:alpine version older than the 1.27 deploy').toBeTruthy();

    const rollbackTo = async (version, imageTail) => {
      const res = await apiRequest('POST', '/api/apps/e2e-nginx/rollback', { version_id: version.id });
      expect(res.status).toBe(200);
      // RollbackOne redeploys before responding.
      expect(containerImage(findServiceContainer('e2e-nginx', 'web'))).toMatch(new RegExp(`(^|/)${imageTail}$`));
      const events = await apiRequest('GET', '/api/apps/e2e-nginx/events');
      expect(events.data[0].action).toBe('rollback');
      expect(events.data[0].detail).toContain(`rollback to version ${version.version}`);
      await waitForProxyStatus('nginx-test.local', 200, { bodyContains: 'Welcome to nginx' });
    };

    await rollbackTo(older, 'nginx:alpine');
    // Later specs expect the 1.27 image back.
    await rollbackTo(latest, 'nginx:1.27-alpine');
    await waitForAppSettled('e2e-nginx');
  });
});
