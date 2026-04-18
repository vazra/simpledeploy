// Deploy every app template end-to-end through the wizard. Expensive:
// pulls ~20 different multi-service stacks. NEVER runs under `make e2e`
// or `make e2e-lite` — only when E2E_TEMPLATES=1 (see playwright.config.js
// testMatch gate and the `e2e-templates` Make target). Intended to run
// once whenever a template is added or changed.
//
// Strategy: for each template, fill its declared variables with
// type-appropriate defaults, deploy through the wizard, assert the
// "Deployed" pill, then delete the app via API before the next template.

import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { appTemplates } from '../../ui/src/lib/appTemplates.js';

async function removeAppIfExists(slug) {
  try { await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password); } catch {}
  try { await apiRequest('DELETE', `/api/apps/${slug}`); } catch {}
}

function slugFor(tpl) {
  return `e2e-tpl-${tpl.id}`.slice(0, 40).replace(/[^a-z0-9-]/g, '-');
}

async function fillVariable(dialog, v, tpl) {
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
      value = `${slugFor(tpl)}.local`;
      break;
    case 'email':
      value = 'e2e@example.com';
      break;
    case 'number':
      value = String(v.default ?? v.placeholder ?? 8080);
      break;
    case 'secret':
      value = 'E2eTestSecretValue12345';
      break;
    default:
      value = v.default != null ? String(v.default) : `e2e-${v.key}`;
  }
  await input.fill(value);
}

test.describe('Deploy every template (E2E_TEMPLATES=1)', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  for (const tpl of appTemplates) {
    test(`deploy template "${tpl.name}"`, async ({ page }) => {
      // Budget per template: pulls + boot of multi-service stacks.
      test.setTimeout(600_000);
      const slug = slugFor(tpl);

      await removeAppIfExists(slug);

      const state = getState();
      await page.goto(`${state.baseURL}/#/`);
      await page.getByRole('button', { name: 'Deploy App' }).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await dialog.getByRole('button', { name: `Use template ${tpl.name}` }).click();

      // Expand advanced/secrets so any required hidden vars are reachable.
      const hasHidden = (tpl.variables || []).some((v) => v.hidden);
      if (hasHidden) {
        await dialog.getByText(/advanced \/ secrets/i).click();
      }

      for (const v of tpl.variables || []) {
        await fillVariable(dialog, v, tpl);
      }

      await dialog.getByRole('button', { name: /^apply/i }).click();

      // Step 1: name + compose validation.
      await dialog.getByPlaceholder('my-app').fill(slug);
      await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 20_000 });

      await dialog.getByRole('button', { name: 'Next' }).click();
      await dialog.getByRole('button', { name: 'Deploy' }).click();

      await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({
        timeout: 540_000,
      });

      await removeAppIfExists(slug);
    });
  }
});
