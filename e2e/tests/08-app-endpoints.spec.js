import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { fetchViaProxy, waitForProxyStatus } from '../helpers/proxy.js';

const ORIGINAL_NGINX_ENDPOINTS = [
  { domain: 'nginx-test.local', port: '80', tls: 'off', service: 'web' },
];

async function waitForProxyNotNginx(host, timeoutMs = 15_000) {
  const deadline = Date.now() + timeoutMs;
  let last = '';
  while (Date.now() < deadline) {
    const r = await fetchViaProxy(host, '/');
    const body = await r.text();
    last = body.slice(0, 60);
    if (r.status !== 200 || !body.includes('Welcome to nginx')) return { status: r.status, body };
    await new Promise((res) => setTimeout(res, 300));
  }
  throw new Error(`host ${host} still routed to nginx within ${timeoutMs}ms (last body=${last})`);
}

test.describe('Endpoints', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  // Every test changes e2e-nginx endpoints; later specs need the original
  // route back and serving.
  test.afterEach(async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', ORIGINAL_NGINX_ENDPOINTS);
    expect(res.status).toBe(200);
    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 20_000, bodyContains: 'Welcome to nginx' });
  });

  test('endpoint added in Settings routes through the proxy', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'settings', exact: true }).click();

    const card = page.locator('main div.rounded-xl').filter({ has: page.getByRole('heading', { name: 'Endpoints', exact: true }) });
    const original = card.locator('div.group').filter({ hasText: 'nginx-test.local' });
    await expect(original).toContainText('web:80');
    await expect(original.getByText('No TLS', { exact: true })).toBeVisible();

    await card.getByRole('button', { name: '+ Add' }).click();
    await card.getByPlaceholder('myapp.example.com').fill('added-e2e.local');
    await card.getByPlaceholder('3000').fill('80');
    await card.locator('select').filter({ has: page.locator('option[value="off"]') }).selectOption('off');
    const res = page.waitForResponse((r) => r.url().endsWith('/api/apps/e2e-nginx/endpoints') && r.request().method() === 'PUT');
    await card.getByRole('button', { name: 'Add Endpoint' }).click();
    expect((await res).status()).toBe(200);

    const added = card.locator('div.group').filter({ hasText: 'added-e2e.local' });
    await expect(added).toContainText('web:80');
    await expect(added.getByText('No TLS', { exact: true })).toBeVisible();
    await waitForProxyStatus('added-e2e.local', 200, { timeoutMs: 40_000, bodyContains: 'Welcome to nginx' });
    await waitForProxyStatus('nginx-test.local', 200, { bodyContains: 'Welcome to nginx' });
  });

  test('removed domain stops routing to the app', async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', [
      { domain: 'only-other.local', port: '80', tls: 'off', service: 'web' },
    ]);
    expect(res.status).toBe(200);

    const r = await waitForProxyNotNginx('nginx-test.local', 20_000);
    expect(r.body).not.toContain('Welcome to nginx');
    await waitForProxyStatus('only-other.local', 200, { timeoutMs: 20_000, bodyContains: 'Welcome to nginx' });
  });

  test('multiple endpoints for same service both route to 200', async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', [
      ...ORIGINAL_NGINX_ENDPOINTS,
      { domain: 'alt-e2e.local', port: '80', tls: 'off', service: 'web' },
    ]);
    expect(res.status).toBe(200);

    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 20_000, bodyContains: 'Welcome to nginx' });
    await waitForProxyStatus('alt-e2e.local', 200, { timeoutMs: 20_000, bodyContains: 'Welcome to nginx' });
  });
});
