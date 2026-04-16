import { test, expect } from '@playwright/test';
import { apiRequest, apiLogin } from '../helpers/api.js';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';

test.describe('Env Vars, Endpoints, and IP Access', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.describe('Env Variables (API)', () => {
    test('GET env returns array', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx/env');
      expect(res.status).toBe(200);
      expect(Array.isArray(res.data)).toBe(true);
    });

    test('PUT env with vars succeeds', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/env', [
        { key: 'E2E_TEST', value: 'hello' },
      ]);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });

    test('GET env confirms persistence', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx/env');
      expect(res.status).toBe(200);
      expect(Array.isArray(res.data)).toBe(true);
      const found = res.data.find((v) => v.key === 'E2E_TEST');
      expect(found).toBeDefined();
      expect(found.value).toBe('hello');
    });

    test('PUT env with empty array clears vars', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/env', []);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });

    test('GET env confirms cleared', async () => {
      const res = await apiRequest('GET', '/api/apps/e2e-nginx/env');
      expect(res.status).toBe(200);
      expect(Array.isArray(res.data)).toBe(true);
      expect(res.data.length).toBe(0);
    });
  });

  test.describe('Env Variables (UI)', () => {
    test('settings page has Environment Variables heading', async ({ page }) => {
      await loginAsAdmin(page);
      const state = getState();
      await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
      await page.getByRole('button', { name: 'Settings' }).click();
      await expect(
        page.getByText('Environment Variables', { exact: false }).first()
      ).toBeVisible({ timeout: 10_000 });
    });

    test('settings page has Add Variable button', async ({ page }) => {
      await loginAsAdmin(page);
      const state = getState();
      await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
      await page.getByRole('button', { name: 'Settings' }).click();
      await expect(
        page.getByRole('button', { name: /Add Variable/i }).first()
      ).toBeVisible({ timeout: 10_000 });
    });
  });

  test.describe('Endpoints (API)', () => {
    let originalEndpoints;

    test.beforeAll(async () => {
      // Capture current app endpoints to restore later
      const res = await apiRequest('GET', '/api/apps/e2e-nginx');
      if (res.ok && res.data?.endpoints) {
        originalEndpoints = res.data.endpoints;
      } else {
        originalEndpoints = [{ domain: 'nginx-test.local', port: '80', tls: 'off', service: 'nginx' }];
      }
    });

    test('PUT endpoints with new endpoint succeeds', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', [
        { domain: 'nginx-e2e-updated.local', port: '80', tls: 'off', service: 'nginx' },
      ]);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
      expect(Array.isArray(res.data.endpoints)).toBe(true);
    });

    test('PUT endpoints restores original', async () => {
      const endpoints = originalEndpoints.length > 0
        ? originalEndpoints
        : [{ domain: 'nginx-test.local', port: '80', tls: 'off', service: 'nginx' }];
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', endpoints);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });
  });

  test.describe('IP Access (API)', () => {
    test('PUT access with single IP returns 200', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', {
        allow: '192.168.1.1',
      });
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });

    test('PUT access with CIDR returns 200', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', {
        allow: '10.0.0.0/8',
      });
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });

    test('PUT access with comma-separated IPs and CIDR returns 200', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', {
        allow: '192.168.1.1, 10.0.0.0/8',
      });
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });

    test('PUT access with invalid CIDR returns 400', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', {
        allow: '10.0.0.0/33',
      });
      expect(res.status).toBe(400);
    });

    test('PUT access with empty string clears and returns 200', async () => {
      const res = await apiRequest('PUT', '/api/apps/e2e-nginx/access', {
        allow: '',
      });
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
    });
  });
});
