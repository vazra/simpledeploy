// Slow end-to-end template deploy coverage.
// Picks a few lightweight templates, drives them through the deploy
// wizard to completion, verifies the container is running, then cleans
// up. Excluded from e2e-lite because each template pulls images.
//
// Kept deliberately narrow: covers the template -> wizard -> deploy
// round-trip. Full matrix is redundant with 30-app-templates-ui which
// already validates every template's compose.

import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { findServiceContainer, containerRunning } from '../helpers/docker.js';

// Templates chosen to minimise pull + boot time. Each lists its expected
// primary service name (as derived by the wizard from template id) and
// the domain to use.
const CASES = [
  { id: 'nginx-static', slug: 'e2e-tpl-nginx',  service: 'web',  domain: 'e2e-tpl-nginx.local' },
  { id: 'uptime-kuma',  slug: 'e2e-tpl-uptime', service: 'app',  domain: 'e2e-tpl-uptime.local' },
];

async function removeAppIfExists(slug) {
  try { await apiLogin(); } catch {}
  const res = await apiRequest('DELETE', `/api/apps/${slug}`);
  return res.ok;
}

test.describe('App templates - slow deploy', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  for (const c of CASES) {
    test(`deploy template "${c.id}" via wizard`, async ({ page }) => {
      test.setTimeout(300_000);

      // Ensure clean state in case a previous run left the app around.
      await removeAppIfExists(c.slug);

      const state = getState();
      await page.goto(`${state.baseURL}/#/`);
      await page.getByRole('button', { name: 'Deploy App' }).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.getByRole('button', { name: /^browse templates$/i }).click();

      await dialog.getByRole('button', { name: new RegExp(`use template .+`, 'i') }).first().waitFor();
      // Templates display by name; find the card for this template id by
      // matching its aria-label prefix.
      const card = dialog.locator(`button[aria-label^="Use template "]`).filter({ hasText: new RegExp(c.id.replace(/-/g, '.'), 'i') });
      await card.first().click();

      await dialog.locator('#tpl-var-domain').fill(c.domain);
      await dialog.getByRole('button', { name: /^apply/i }).click();

      // Step 1: name it
      await dialog.getByPlaceholder('my-app').fill(c.slug);
      await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 15_000 });
      await dialog.getByRole('button', { name: 'Next' }).click();
      await dialog.getByRole('button', { name: 'Deploy' }).click();

      // Deploy can be slow on first pull.
      await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 240_000 });

      // Verify the container actually exists and is running.
      const container = findServiceContainer(c.slug, c.service);
      expect(container, `expected ${c.slug}/${c.service} container`).toBeTruthy();
      expect(containerRunning(container)).toBe(true);

      await removeAppIfExists(c.slug);
    });
  }
});
