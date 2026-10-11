import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { b64 } from '../helpers/users.js';

const VALID_COMPOSE = b64('services:\n  web:\n    image: nginx:alpine\n');
// Each rule is unit-tested in internal/compose; this proves the real binary
// wires the checks into deploy and validate-compose.
const UNSAFE_COMPOSE = b64('services:\n  web:\n    image: nginx:alpine\n    privileged: true\n    network_mode: host\n');

// A rejected deploy may have started anyway (expected-fail runs): let it
// settle, then remove it so later specs see a clean app list.
async function removeIfStarted(res, name) {
  if (res.status !== 202) return;
  await waitForAppStatus(name, 'error', 90_000);
  expect((await apiRequest('DELETE', `/api/apps/${encodeURIComponent(name)}`)).status).toBe(200);
}

test.describe('Security Validation', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test.describe('Compose Security', () => {
    test('deploy rejects unsafe compose with every violation and persists nothing', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', { name: 'sec-test-multi', compose: UNSAFE_COMPOSE });
      expect(res.status).toBe(400);
      expect(res.data.violations.some((v) => v.includes('privileged'))).toBe(true);
      expect(res.data.violations.some((v) => v.includes('network_mode'))).toBe(true);
      expect((await apiRequest('GET', '/api/apps/sec-test-multi')).status).toBe(404);
      expect((await apiRequest('GET', '/api/apps/sec-test-multi/compose')).status).toBe(404);
    });

    test('validate-compose reports the violations deploy would reject', async () => {
      const res = await apiRequest('POST', '/api/apps/validate-compose', { compose: UNSAFE_COMPOSE });
      expect(res.status).toBe(200);
      expect(res.data.valid).toBe(false);
      expect(JSON.stringify(res.data.errors)).toContain('privileged');
    });
  });

  test.describe('App Name Validation', () => {
    test('rejects invalid app names with 400', async () => {
      const cases = [
        ['', 'name and compose are required'],
        ['-invalid', 'invalid app name'],
        ['.invalid', 'invalid app name'],
        ['..', 'invalid app name'],
        ['../evil', 'invalid app name'],
        ['my app', 'invalid app name'],
        ['a' + 'b'.repeat(63), 'invalid app name'],
      ];
      for (const [name, msg] of cases) {
        const res = await apiRequest('POST', '/api/apps/deploy', { name, compose: VALID_COMPOSE });
        expect(res.status, `name ${JSON.stringify(name)}`).toBe(400);
        expect(res.data, `name ${JSON.stringify(name)}`).toContain(msg);
      }
    });

    test('rejects names docker compose cannot use as a project name (dots, uppercase)', async () => {
      for (const name of ['my-app_v1.0', 'MyApp']) {
        const res = await apiRequest('POST', '/api/apps/deploy', { name, compose: VALID_COMPOSE });
        try {
          expect(res.status, `name ${name}`).toBe(400);
          expect(res.data, `name ${name}`).toContain('invalid app name');
        } finally {
          await removeIfStarted(res, name);
        }
      }
    });

    test('a 63-char name with dashes and underscores deploys and runs', async () => {
      const name = 'e2e-long_name-'.padEnd(63, 'x');
      expect(name).toHaveLength(63);
      const res = await apiRequest('POST', '/api/apps/deploy', { name, compose: VALID_COMPOSE });
      expect(res.status).toBe(202);
      try {
        await waitForAppStatus(name, 'running', 90_000);
      } finally {
        expect((await apiRequest('DELETE', `/api/apps/${name}`)).status).toBe(200);
      }
    });

    test('rejects missing compose field', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', { name: 'test-app', compose: '' });
      expect(res.status).toBe(400);
      expect(res.data).toContain('name and compose are required');
    });
  });
});
