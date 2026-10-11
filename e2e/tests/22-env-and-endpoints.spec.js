import { test, expect } from '@playwright/test';
import { apiRequest, apiLogin } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { waitForProxyStatus } from '../helpers/proxy.js';
import { waitForAppSettled } from '../helpers/users.js';

const HOST = 'nginx-test.local';

test.describe('Env Vars and IP Access', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test('env vars round-trip through the app .env and clear', async () => {
    const put = await apiRequest('PUT', '/api/apps/e2e-nginx/env', [{ key: 'E2E_TEST', value: 'hello' }]);
    expect(put.status).toBe(200);
    expect(put.data.status).toBe('ok');
    const got = await apiRequest('GET', '/api/apps/e2e-nginx/env');
    expect(got.status).toBe(200);
    expect(got.data).toEqual([expect.objectContaining({ key: 'E2E_TEST', value: 'hello' })]);

    const clear = await apiRequest('PUT', '/api/apps/e2e-nginx/env', []);
    expect(clear.status).toBe(200);
    const cleared = await apiRequest('GET', '/api/apps/e2e-nginx/env');
    expect(cleared.data).toEqual([]);
  });

  test('invalid CIDR in the IP allowlist is rejected', async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', { allow: '10.0.0.0/33' });
    expect(res.status).toBe(400);
  });

  test('IP allowlist is enforced by the proxy: deny, allow, then cleared', async () => {
    // Each change rewrites the compose label; the watcher then redeploys the
    // app, so wait for that before the next change.
    const setAllow = async (allow) => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', { allow });
      expect(res.status).toBe(200);
      await waitForAppSettled('e2e-nginx');
    };
    try {
      // Requests arrive from 127.0.0.1, which this list does not cover: 404.
      await setAllow('192.168.1.1, 10.0.0.0/8');
      await waitForProxyStatus(HOST, 404);
      await setAllow('127.0.0.0/8');
      await waitForProxyStatus(HOST, 200, { bodyContains: 'Welcome to nginx' });
    } finally {
      // Later specs need nginx-test.local reachable without an allowlist.
      await setAllow('');
      const compose = await apiRequest('GET', '/api/apps/e2e-nginx/compose');
      expect(compose.data).not.toMatch(/simpledeploy\.access\.allow:\s*["']?[0-9]/);
      await waitForProxyStatus(HOST, 200, { bodyContains: 'Welcome to nginx' });
    }
  });
});
