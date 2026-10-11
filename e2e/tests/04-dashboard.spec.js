import { test, expect } from '@playwright/test';
import { loginAsAdmin, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

const APPS = ['e2e-nginx', 'e2e-multi', 'e2e-postgres'];

test.describe('Dashboard', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test('shows app status, host metrics, search and card navigation', async ({ page }) => {
    // Host metrics flow collector -> /api/metrics/system -> stat cards. Wait
    // for the first system point so the page load below must render it.
    await expect.poll(async () => {
      const res = await apiRequest('GET', '/api/metrics/system?range=1h');
      return res.data?.containers?.['']?.points?.length || 0;
    }, { timeout: 30_000 }).toBeGreaterThan(0);

    await loginAsAdmin(page);
    const main = page.locator('main');

    for (const slug of APPS) {
      const card = main.locator(`a[href="#/apps/${slug}"]`);
      await expect(card.getByRole('heading', { name: slug, exact: true })).toBeVisible();
      await expect(card.getByText('running', { exact: true })).toBeVisible();
    }
    const apps = (await apiRequest('GET', '/api/apps')).data;
    const running = apps.filter((a) => a.Status === 'running').length;
    await expect(main.getByRole('button', { name: /^Running/ }).locator('span').last()).toHaveText(String(running));

    // Memory stat card sub-text reads "<used> / <total>"; both stay "0" without data.
    const memCard = main.getByText('Memory', { exact: true }).locator('xpath=../..');
    await expect(memCard.getByText(/^\d+\.\d [KMGT]?B \/ \d+\.\d [KMGT]?B$/)).toBeVisible();

    await main.getByPlaceholder(/search/i).fill('nginx');
    await expect(main.getByRole('heading', { name: 'e2e-nginx', exact: true })).toBeVisible();
    await expect(main.getByRole('heading', { name: 'e2e-multi', exact: true })).toHaveCount(0);
    await expect(main.getByRole('heading', { name: 'e2e-postgres', exact: true })).toHaveCount(0);

    await main.getByRole('heading', { name: 'e2e-nginx', exact: true }).click();
    await expect(page).toHaveURL(/#\/apps\/e2e-nginx$/);
    await expect(page.locator('h1')).toHaveText('e2e-nginx');
  });
});
