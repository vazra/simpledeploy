import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';

test.describe('Edge Cases', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.describe('Audit Log', () => {
    test('GET /api/system/audit-log returns array', async () => {
      const res = await apiRequest('GET', '/api/system/audit-log');
      expect(res.status).toBe(200);
      expect(Array.isArray(res.data)).toBe(true);
    });

    test('audit log has at least one login event', async () => {
      const res = await apiRequest('GET', '/api/system/audit-log');
      expect(res.status).toBe(200);
      const loginEvents = res.data.filter((e) => e.type === 'login');
      expect(loginEvents.length).toBeGreaterThan(0);
    });

    test('audit log events have required fields', async () => {
      const res = await apiRequest('GET', '/api/system/audit-log');
      expect(res.status).toBe(200);
      expect(res.data.length).toBeGreaterThan(0);
      const event = res.data[0];
      expect(event).toHaveProperty('timestamp');
      expect(event).toHaveProperty('type');
      expect(event).toHaveProperty('username');
    });
  });

  test.describe('Alert Rule CRUD', () => {
    let webhookId = null;
    let ruleId = null;

    test('create webhook for alert rule tests', async () => {
      const res = await apiRequest('POST', '/api/webhooks', {
        name: 'e2e-edge-webhook',
        type: 'custom',
        url: 'https://example.com/webhook',
      });
      expect(res.status).toBe(201);
      expect(res.data.id).toBeTruthy();
      webhookId = res.data.id;
    });

    test('create alert rule', async () => {
      expect(webhookId).toBeTruthy();
      const res = await apiRequest('POST', '/api/alerts/rules', {
        metric: 'cpu_pct',
        operator: '>',
        threshold: 80,
        duration_sec: 60,
        webhook_id: webhookId,
        enabled: true,
      });
      expect(res.status).toBe(201);
      expect(res.data.id).toBeTruthy();
      expect(res.data.threshold).toBe(80);
      ruleId = res.data.id;
    });

    test('update alert rule threshold', async () => {
      expect(ruleId).toBeTruthy();
      const res = await apiRequest('PUT', `/api/alerts/rules/${ruleId}`, {
        metric: 'cpu_pct',
        operator: '>',
        threshold: 90,
        duration_sec: 60,
        webhook_id: webhookId,
        enabled: true,
      });
      expect(res.status).toBe(200);
      expect(res.data.threshold).toBe(90);
    });

    test('GET /api/alerts/rules reflects updated threshold', async () => {
      expect(ruleId).toBeTruthy();
      const res = await apiRequest('GET', '/api/alerts/rules');
      expect(res.status).toBe(200);
      const rule = res.data.find((r) => r.id === ruleId);
      expect(rule).toBeTruthy();
      expect(rule.threshold).toBe(90);
    });

    test('delete alert rule', async () => {
      expect(ruleId).toBeTruthy();
      const res = await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
      ruleId = null;
    });

    test('delete webhook', async () => {
      expect(webhookId).toBeTruthy();
      const res = await apiRequest('DELETE', `/api/webhooks/${webhookId}`);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
      webhookId = null;
    });
  });

  test.describe('Webhook CRUD', () => {
    let webhookId = null;

    test('create webhook', async () => {
      const res = await apiRequest('POST', '/api/webhooks', {
        name: 'e2e-webhook-edit',
        type: 'custom',
        url: 'https://example.com/original',
      });
      expect(res.status).toBe(201);
      webhookId = res.data.id;
    });

    test('update webhook URL', async () => {
      expect(webhookId).toBeTruthy();
      const res = await apiRequest('PUT', `/api/webhooks/${webhookId}`, {
        name: 'e2e-webhook-edit',
        type: 'custom',
        url: 'https://example.com/updated',
      });
      expect(res.status).toBe(200);
      expect(res.data.url).toBe('https://example.com/updated');
    });

    test('GET /api/webhooks reflects updated URL', async () => {
      expect(webhookId).toBeTruthy();
      const res = await apiRequest('GET', '/api/webhooks');
      expect(res.status).toBe(200);
      const wh = res.data.find((w) => w.id === webhookId);
      expect(wh).toBeTruthy();
      expect(wh.url).toBe('https://example.com/updated');
    });

    test('delete webhook', async () => {
      expect(webhookId).toBeTruthy();
      const res = await apiRequest('DELETE', `/api/webhooks/${webhookId}`);
      expect(res.status).toBe(200);
      expect(res.data.status).toBe('ok');
      webhookId = null;
    });
  });

  test.describe('Dashboard Filtering (UI)', () => {
    test('dashboard has status filter, sort, and search controls', async ({ page }) => {
      await loginAsAdmin(page);
      const state = getState();
      await page.goto(`${state.baseURL}/#/`);
      await expect(page.getByText('Applications')).toBeVisible({ timeout: 10_000 });

      // Status filter dropdown (All/Running/Stopped)
      const statusFilter = page.locator('select').filter({ hasText: /all|running|stopped/i }).first();
      await expect(statusFilter).toBeVisible();

      // Sort dropdown
      const sortDropdown = page.locator('select').nth(1);
      await expect(sortDropdown).toBeVisible();

      // Search input
      const searchInput = page.locator('input[type="search"], input[placeholder*="search" i], input[placeholder*="filter" i]').first();
      await expect(searchInput).toBeVisible();
    });
  });

  test.describe('System Prune', () => {
    test('POST /api/system/prune/metrics returns 200', async () => {
      const res = await apiRequest('POST', '/api/system/prune/metrics', {
        tier: 'raw',
        days: 1,
      });
      expect(res.status).toBe(200);
      expect(res.data).toHaveProperty('deleted');
      expect(res.data).toHaveProperty('message');
    });

    test('POST /api/system/prune/request-stats returns 200', async () => {
      const res = await apiRequest('POST', '/api/system/prune/request-stats', {
        tier: 'raw',
        days: 1,
      });
      expect(res.status).toBe(200);
      expect(res.data).toHaveProperty('deleted');
      expect(res.data).toHaveProperty('message');
    });
  });
});
