import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { findServiceContainer, containerImage } from '../helpers/docker.js';

test.describe('Deploy History & Rollback', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.locator('button').filter({ hasText: 'settings' }).click();
  });

  test('shows deploy history', async ({ page }) => {
    // Deploy History is a collapsible section with text "Deploy History (N)"
    // It only appears if versions > 0; wait for config tab to finish loading
    const historyBtn = page.locator('button').filter({ hasText: /Deploy History/i });
    await expect(historyBtn).toBeVisible({ timeout: 10_000 });
  });

  test('deploy history has entries', async ({ page }) => {
    // Expand the deploy history section
    const historyBtn = page.locator('button').filter({ hasText: /Deploy History/i });
    await historyBtn.click();
    // Entries now display as timeline items with "v1", "v2", etc. in a span.
    await expect(page.locator('span').filter({ hasText: /^v\d+$/ }).first()).toBeVisible({ timeout: 5_000 });
  });
});

test.describe('Versions - Functional', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test('new deployment creates new version with updated image', async () => {
    // Baseline image (should be nginx:alpine from original fixture)
    const containerBefore = findServiceContainer('e2e-nginx', 'web');
    expect(containerBefore, 'expected e2e-nginx web container').toBeTruthy();
    const imageBefore = containerImage(containerBefore);
    expect(imageBefore).toContain('nginx');

    // Version count before redeploy
    const beforeRes = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
    expect(beforeRes.ok).toBeTruthy();
    const countBefore = beforeRes.data.length;

    // Redeploy with pinned tag nginx:1.27-alpine. Deploy endpoint accepts base64 compose.
    const newCompose = [
      'services:',
      '  web:',
      '    image: nginx:1.27-alpine',
      '    ports:',
      '      - "8091:80"',
      '    labels:',
      '      simpledeploy.endpoints.0.domain: "nginx-test.local"',
      '      simpledeploy.endpoints.0.port: "80"',
      '      simpledeploy.endpoints.0.tls: "off"',
      '',
    ].join('\n');
    const b64 = Buffer.from(newCompose, 'utf-8').toString('base64');
    const deployRes = await apiRequest('POST', '/api/apps/deploy', {
      name: 'e2e-nginx',
      compose: b64,
      force: true,
    });
    expect(deployRes.status).toBe(202);

    // Wait for app to be running with new image (poll up to 60s)
    const deadline = Date.now() + 90_000;
    let currentImage = imageBefore;
    let containerNow = containerBefore;
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2_000));
      containerNow = findServiceContainer('e2e-nginx', 'web');
      if (containerNow) {
        try {
          currentImage = containerImage(containerNow);
          if (currentImage.includes('1.27')) break;
        } catch {}
      }
    }
    expect(currentImage).toContain('nginx:1.27-alpine');

    // Wait for app status to stabilise before asserting versions
    await waitForAppStatus('e2e-nginx', 'running', 60_000).catch(() => {});

    // Versions list now has at least countBefore+1 (and newest-first)
    const afterRes = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
    expect(afterRes.ok).toBeTruthy();
    expect(Array.isArray(afterRes.data)).toBe(true);
    expect(afterRes.data.length).toBeGreaterThanOrEqual(countBefore + 1);
    expect(afterRes.data.length).toBeGreaterThanOrEqual(2);

    // Newest first: compare first two timestamps (ms precision)
    if (afterRes.data.length >= 2) {
      const t0 = new Date(afterRes.data[0].created_at).getTime();
      const t1 = new Date(afterRes.data[1].created_at).getTime();
      expect(t0).toBeGreaterThanOrEqual(t1);
    }

    // Latest version's compose contains the new image tag
    const latest = afterRes.data[0];
    expect(typeof latest.content).toBe('string');
    expect(latest.content).toContain('nginx:1.27-alpine');
  });

  test('version compose content is retrievable and valid', async () => {
    const listRes = await apiRequest('GET', '/api/apps/e2e-nginx/versions');
    expect(listRes.ok).toBeTruthy();
    expect(listRes.data.length).toBeGreaterThanOrEqual(1);
    const latest = listRes.data[0];

    // Inline content from list
    expect(typeof latest.content).toBe('string');
    expect(latest.content.length).toBeGreaterThan(0);
    expect(latest.content).toMatch(/services\s*:/);
    expect(latest.content).toContain('web');

    // Download endpoint returns raw YAML (apiRequest falls through to text on non-JSON)
    const dlRes = await apiRequest('GET', `/api/apps/e2e-nginx/versions/${latest.id}/download`);
    expect(dlRes.ok).toBeTruthy();
    const body = typeof dlRes.data === 'string' ? dlRes.data : JSON.stringify(dlRes.data);
    expect(body).toMatch(/services\s*:/);
    expect(body).toContain('web');
  });
});
