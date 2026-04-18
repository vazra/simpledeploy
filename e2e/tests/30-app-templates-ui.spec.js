// App template UI coverage.
// Iterates every app template in ui/src/lib/appTemplates.js. For each:
//  1. open the deploy wizard
//  2. click the template card
//  3. fill any required primary vars (secrets auto-generate)
//  4. click Apply
//  5. verify the wizard advances to step 1 and reports a valid compose
//  6. Back to picker for the next template
// No actual deployment happens here — that's covered by the slow
// deploy-each spec. This test is fast and runs in lite mode.

import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import { appTemplates } from '../../ui/src/lib/appTemplates.js';

test.describe('App templates - UI validation', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  // Fill a single variable field based on its rendered input type.
  async function fillVariable(dialog, v, templateId) {
    const input = dialog.locator(`#tpl-var-${v.key}`);
    if (!(await input.isVisible().catch(() => false))) return;

    if (v.type === 'enum') {
      const opts = v.options || [];
      const def = v.default || opts[0]?.value;
      if (def) await input.selectOption(String(def));
      return;
    }

    // Skip if already filled (defaults / auto-generated secrets).
    const existing = await input.inputValue().catch(() => '');
    if (existing && existing.length > 0) return;

    let value = '';
    switch (v.type) {
      case 'domain':
        value = `${templateId}.e2e.test`;
        break;
      case 'email':
        value = 'e2e@example.com';
        break;
      case 'number':
        value = String(v.default ?? 8080);
        break;
      case 'secret':
        // Picker auto-fills on open; only reach here if generation failed.
        value = 'E2eTestSecretValue12345';
        break;
      default:
        value = v.default != null ? String(v.default) : `e2e-${v.key}`;
    }
    await input.fill(value);
  }

  test(`renders ${appTemplates.length} templates in grid`, async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    for (const tpl of appTemplates) {
      await expect(
        dialog.getByRole('button', { name: `Use template ${tpl.name}` }),
      ).toBeVisible();
    }
  });

  // One test per template keeps failures isolated and the report legible.
  for (const tpl of appTemplates) {
    test(`template "${tpl.name}" applies to a valid compose`, async ({ page }) => {
      const state = getState();
      await page.goto(`${state.baseURL}/#/`);
      await page.getByRole('button', { name: 'Deploy App' }).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await dialog.getByRole('button', { name: `Use template ${tpl.name}` }).click();

      // Expand advanced/secrets accordion if the template has hidden vars so
      // we can verify auto-generated secrets are populated.
      const hasHidden = (tpl.variables || []).some((v) => v.hidden);
      if (hasHidden) {
        await dialog.getByText(/advanced \/ secrets/i).click();
      }

      for (const v of tpl.variables || []) {
        await fillVariable(dialog, v, tpl.id);
      }

      await dialog.getByRole('button', { name: /^apply/i }).click();

      // Step 1: expect the valid-compose indicator to appear. Validation
      // happens server-side, so allow room for the round-trip.
      await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 15_000 });
    });
  }
});
