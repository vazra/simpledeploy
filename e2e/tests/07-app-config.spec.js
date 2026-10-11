import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, waitForDeployDone } from '../helpers/api.js';
import { findServiceContainer, containerImage } from '../helpers/docker.js';
import { waitForProxyStatus } from '../helpers/proxy.js';

test.describe('App Configuration', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  // Leaves e2e-nginx on nginx:1.27-alpine with a second compose version,
  // which 11-app-versions and 23-rollback-and-restore build on.
  test('Save & Deploy from the YAML editor redeploys with the edited image', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'settings', exact: true }).click();
    const main = page.locator('main');
    await main.getByRole('button', { name: 'YAML', exact: true }).click();

    const editor = main.locator('textarea').first();
    await expect(editor).toHaveValue(/image: nginx:alpine\n/);
    const edited = (await editor.inputValue()).replace('image: nginx:alpine', 'image: nginx:1.27-alpine');
    await editor.fill(edited);
    await main.getByRole('button', { name: 'Save & Deploy' }).click();

    const review = page.getByRole('dialog').filter({ hasText: 'Review Changes' });
    await expect(review.getByText(/^- +image: nginx:alpine$/)).toBeVisible();
    await expect(review.getByText(/^\+ +image: nginx:1\.27-alpine$/)).toBeVisible();
    const deployRes = page.waitForResponse((r) => r.url().endsWith('/api/apps/deploy') && r.request().method() === 'POST');
    await review.getByRole('button', { name: 'Confirm & Deploy' }).click();
    expect((await deployRes).status()).toBe(202);
    await expect(review).toBeHidden();

    await expect.poll(() => {
      const name = findServiceContainer('e2e-nginx', 'web');
      return name ? containerImage(name) : '';
    }, { timeout: 90_000 }).toMatch(/(^|\/)nginx:1\.27-alpine$/);
    expect((await waitForDeployDone('e2e-nginx')).Status).toBe('running');
    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 30_000, bodyContains: 'Welcome to nginx' });
  });
});
