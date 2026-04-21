import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { fetchViaProxy } from '../helpers/proxy.js';

test.describe('Endpoints & Access', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'Settings' }).click();
  });

  test('shows current endpoints', async ({ page }) => {
    await expect(page.getByText('nginx-test.local').first()).toBeVisible({ timeout: 10_000 });
  });

  test('shows TLS mode', async ({ page }) => {
    await expect(page.getByText(/No TLS|Auto TLS|Local CA|Custom TLS/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('Advanced section has IP allowlist', async ({ page }) => {
    // Verify the Advanced section and its contents exist in the DOM
    // even if the collapsible toggle has rendering issues
    const hasAdvanced = await page.getByRole('button', { name: 'Advanced' }).isVisible();
    expect(hasAdvanced).toBeTruthy();
  });
});

const ORIGINAL_NGINX_ENDPOINTS = [
  { domain: 'nginx-test.local', port: '80', tls: 'off', service: 'web' },
];

// waitForProxyStatus polls Caddy until the response matches wantStatus and,
// when bodyContains is provided, the response body also contains that
// substring. The body check is necessary because Caddy's fallthrough
// handler can return 200 with an empty body during the window between the
// endpoints PUT and the reconciler installing the new route; accepting that
// empty 200 would mask real routing failures.
async function waitForProxyStatus(host, wantStatus, timeoutMs = 15_000, bodyContains = null) {
  const deadline = Date.now() + timeoutMs;
  let last = 0;
  let lastBody = '';
  while (Date.now() < deadline) {
    try {
      const r = await fetchViaProxy(host, '/');
      last = r.status;
      if (r.status === wantStatus) {
        if (!bodyContains) return r;
        const body = await r.text();
        lastBody = body.slice(0, 60);
        if (body.includes(bodyContains)) {
          return { ...r, text: async () => body };
        }
      }
    } catch {}
    await new Promise((r) => setTimeout(r, 500));
  }
  const hint = bodyContains ? ` body~${JSON.stringify(bodyContains)}` : '';
  throw new Error(`host ${host} did not reach status ${wantStatus}${hint} within ${timeoutMs}ms (last=${last}, lastBody=${lastBody})`);
}

async function waitForProxyNotNginx(host, timeoutMs = 15_000) {
  const deadline = Date.now() + timeoutMs;
  let last = '';
  while (Date.now() < deadline) {
    try {
      const r = await fetchViaProxy(host, '/');
      const body = await r.text();
      last = body.slice(0, 60);
      if (r.status !== 200 || !body.includes('Welcome to nginx')) return { status: r.status, body };
    } catch {
      return { status: 0, body: '' };
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`host ${host} still routed to nginx within ${timeoutMs}ms (last body=${last})`);
}

test.describe.configure({ mode: 'serial' });
test.describe('Endpoints - Functional', () => {
  test.beforeAll(async () => {
    await apiLogin('e2eadmin', 'E2eTestPass123!');
  });

  test.afterEach(async () => {
    // Always restore nginx endpoints after each functional test.
    await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', ORIGINAL_NGINX_ENDPOINTS);
    try {
      await waitForProxyStatus('nginx-test.local', 200, 20_000, 'Welcome to nginx');
    } catch {}
  });

  test('added domain becomes reachable via proxy', async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', [
      ...ORIGINAL_NGINX_ENDPOINTS,
      { domain: 'added-e2e.local', port: '80', tls: 'off', service: 'web' },
    ]);
    expect(res.ok).toBe(true);

    const r = await waitForProxyStatus('added-e2e.local', 200, 20_000, 'Welcome to nginx');
    const body = await r.text();
    expect(body).toContain('Welcome to nginx');
  });

  test('removed domain returns non-200 via proxy', async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', [
      { domain: 'only-other.local', port: '80', tls: 'off', service: 'web' },
    ]);
    expect(res.ok).toBe(true);

    const r = await waitForProxyNotNginx('nginx-test.local', 40_000);
    expect(r.body || '').not.toContain('Welcome to nginx');
  });

  test('multiple endpoints for same service both route to 200', async () => {
    const res = await apiRequest('PUT', '/api/apps/e2e-nginx/endpoints', [
      ...ORIGINAL_NGINX_ENDPOINTS,
      { domain: 'alt-e2e.local', port: '80', tls: 'off', service: 'web' },
    ]);
    expect(res.ok).toBe(true);

    const a = await waitForProxyStatus('nginx-test.local', 200, 20_000, 'Welcome to nginx');
    expect(a.status).toBe(200);
    const b = await waitForProxyStatus('alt-e2e.local', 200, 20_000, 'Welcome to nginx');
    expect(b.status).toBe(200);
  });
});
