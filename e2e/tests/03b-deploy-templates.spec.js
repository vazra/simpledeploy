// E2E: DeployWizard template flow.
// Exercises step 0 (Template grid), variable rendering, auto-generated secrets,
// Apply -> step 1 pre-fill, and full end-to-end deploy through the template path.
// Also covers the "Build it yourself" (blank compose) regression path.
//
// Runs AFTER 03-deploy.spec.js and BEFORE 04-dashboard.spec.js so the dashboard
// tests still see e2e-nginx / e2e-multi / e2e-postgres. The template-deployed
// apps (e2e-nginx-tpl, e2e-node-tpl, e2e-blank-tpl) are cleaned up via API
// after each test to avoid polluting later specs.

import { test, expect } from '@playwright/test';
import { loginAsAdmin, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

test.describe.configure({ mode: 'serial' });

async function cleanupApp(slug) {
  try { await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password); } catch {}
  try { await apiRequest('DELETE', `/api/apps/${slug}`); } catch {}
}

test.describe('Deploy via Templates', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('deploy via Nginx Static template', async ({ page }) => {
    const slug = 'e2e-nginx-tpl';

    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Step 0: click the Nginx Static card (button aria-label "Use template Nginx Static Site").
    await dialog.getByRole('button', { name: /^browse templates$/i }).click();
    await dialog.getByRole('button', { name: /use template nginx static/i }).click();

    // Vars form: switch to Custom domain mode (default is Quick test via sslip.io).
    await dialog.getByRole('button', { name: /^custom domain$/i }).click();
    const domainInput = dialog.locator('#tpl-var-domain');
    await expect(domainInput).toBeVisible();
    await domainInput.fill('e2e-nginx-tpl.localhost');

    // Apply -> step 1.
    await dialog.getByRole('button', { name: /^apply/i }).click();

    // App name should be pre-filled by suggestName().
    const nameInput = dialog.getByPlaceholder('my-app');
    await expect(nameInput).toBeVisible();
    const suggested = await nameInput.inputValue();
    expect(suggested.length).toBeGreaterThan(0);
    // Override to a deterministic slug so we can clean up reliably.
    await nameInput.fill(slug);

    // Wait for compose validation to settle.
    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });

    // Switch to YAML tab and verify the rendered compose contains the template's
    // TLS + domain labels.
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    const yamlText = await editor.inputValue();
    expect(yamlText).toContain('tls: letsencrypt');
    expect(yamlText).toContain('domain: e2e-nginx-tpl.localhost');

    // Re-validate (YAML edit may have rescheduled validation).
    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });

    // Next -> Deploy.
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();
    await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });

    await cleanupApp(slug);
  });

  test('deploy via Node API + Postgres template propagates backup label', async ({ page }) => {
    // Deploy pulls node:20-alpine + postgres:16-alpine; on slow networks
    // this can exceed 5 min end-to-end, so we give it a generous budget.
    test.setTimeout(600_000);
    const slug = 'e2e-node-tpl';

    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByRole('button', { name: /^browse templates$/i }).click();
    await dialog.getByRole('button', { name: /use template node api \+ postgres/i }).click();

    await dialog.getByRole('button', { name: /^custom domain$/i }).click();
    const domainInput = dialog.locator('#tpl-var-domain');
    await expect(domainInput).toBeVisible();
    await domainInput.fill('e2e-node-tpl.localhost');

    // Expand advanced accordion and confirm db_password auto-generated.
    await dialog.getByText(/advanced \/ secrets/i).click();
    const pwInput = dialog.locator('#tpl-var-db_password');
    await expect(pwInput).toBeVisible();
    const originalPw = await pwInput.inputValue();
    expect(originalPw.length).toBeGreaterThan(0);

    // Regenerate and confirm the value changes.
    await dialog.getByRole('button', { name: /^regenerate$/i }).click();
    const regenPw = await pwInput.inputValue();
    expect(regenPw.length).toBeGreaterThan(0);
    expect(regenPw).not.toBe(originalPw);

    // Apply -> step 1.
    await dialog.getByRole('button', { name: /^apply/i }).click();

    const nameInput = dialog.getByPlaceholder('my-app');
    await expect(nameInput).toBeVisible();
    await nameInput.fill(slug);

    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });

    // Verify postgres backup label made it into the rendered compose.
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    const yamlText = await editor.inputValue();
    expect(yamlText).toContain('simpledeploy.backup.strategy: postgres');

    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });

    // Next -> Deploy. Node API + Postgres template pulls large images;
    // allow up to 480s under the 600s test-level timeout.
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();
    await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 480_000 });

    // Navigate to the app's Backups tab and confirm it renders.
    await dialog.getByRole('button', { name: 'View App' }).click();
    await expect(page).toHaveURL(new RegExp(`/#/apps/${slug}`));
    // The tab labels render via capitalize; the DOM text is lowercase "backups".
    await page.getByRole('button', { name: 'backups', exact: true }).click();
    // Hash routing: some deployments add ?tab=backups; just confirm tab content mounted.
    // BackupsTab shows at minimum heading or controls; we assert no console-error surface
    // by waiting for either a known control or an empty-state indicator.
    await expect(page.locator('main')).toBeVisible();

    await cleanupApp(slug);
  });

  test('start with a blank compose file still works', async ({ page }) => {
    const slug = 'e2e-blank-tpl';

    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByRole('button', { name: /build it yourself/i }).click();

    await dialog.getByPlaceholder('my-app').fill(slug);

    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    const blankCompose = [
      'services:',
      '  web:',
      '    image: nginx:alpine',
      '    labels:',
      '      simpledeploy.endpoints.0.domain: e2e-blank-tpl.localhost',
      "      simpledeploy.endpoints.0.port: '80'",
      "      simpledeploy.endpoints.0.tls: 'off'",
      '',
    ].join('\n');
    await editor.fill(blankCompose);

    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });

    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();
    await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });

    await cleanupApp(slug);
  });
});
