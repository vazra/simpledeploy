import { test, expect } from '@playwright/test';
import { existsSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { listAppContainers } from '../helpers/docker.js';

// Removes the apps 03-deploy created (e2e-nginx is purged by 95-archive),
// checking that a delete really tears the app down.
test.describe('Cleanup', () => {
  test.beforeEach(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test('Delete App in Settings removes the app, its containers and its folder', async ({ page }) => {
    const state = getState();
    const slug = 'e2e-postgres';
    expect(listAppContainers(slug).length).toBeGreaterThan(0);

    await loginAsAdmin(page);
    await page.goto(`${state.baseURL}/#/apps/${slug}`);
    await page.locator('main').getByRole('button', { name: 'settings', exact: true }).click();
    await page.getByRole('button', { name: 'Danger Zone' }).click();
    await page.getByRole('button', { name: 'Delete App' }).click();

    const removed = page.waitForResponse((r) => r.url().endsWith(`/api/apps/${slug}`) && r.request().method() === 'DELETE');
    await page.getByRole('dialog').getByRole('button', { name: 'Confirm' }).click();
    expect((await removed).status()).toBe(200);
    await expect(page).toHaveURL(`${state.baseURL}/#/`);
    await expect(page.locator('main').getByRole('heading', { name: slug, exact: true })).toHaveCount(0);

    expect((await apiRequest('GET', `/api/apps/${slug}`)).status).toBe(404);
    expect(listAppContainers(slug)).toEqual([]);
    expect(existsSync(join(state.appsDir, slug))).toBe(false);
  });

  test('DELETE removes every service of a multi-service app', async () => {
    const slug = 'e2e-multi';
    const services = () => listAppContainers(slug).map((n) => n.replace(/^simpledeploy-e2e-multi-(\w+)-\d+$/, '$1')).sort();
    expect([...new Set(services())]).toEqual(['cache', 'web']);

    const res = await apiRequest('DELETE', `/api/apps/${slug}`);
    expect(res.status).toBe(200);
    expect(listAppContainers(slug)).toEqual([]);
    expect((await apiRequest('GET', `/api/apps/${slug}`)).status).toBe(404);
  });
});
