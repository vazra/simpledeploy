// Service template UI coverage.
// Iterates every service template from ui/src/lib/serviceTemplates.js.
// For each, opens the deploy wizard in blank-compose mode, clicks
// "Add Service", picks the template, then confirms the rendered YAML
// contains the template's image reference.
// No deployment happens — this is a fast validator that lives in lite
// mode.

import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import { serviceTemplates } from '../../ui/src/lib/serviceTemplates.js';

test.describe('Service templates - UI validation', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  async function openBlankWizard(page) {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /start with a blank compose/i }).click();
    return dialog;
  }

  for (const tpl of serviceTemplates) {
    test(`service template "${tpl.name}" adds a service with the expected image`, async ({ page }) => {
      const dialog = await openBlankWizard(page);

      // Wizard opens in visual mode by default; open the service templates
      // picker and pick this template. The button's accessible name is the
      // concatenation of its icon emoji and the template name, so partial
      // match on name suffices.
      await dialog.getByRole('button', { name: 'Add Service' }).click();
      await dialog.getByRole('button', { name: tpl.name }).first().click();

      // Switch to YAML and verify the template's image made it in (blank
      // template renders no image, so only assert when one is configured).
      await dialog.getByRole('button', { name: 'YAML' }).click();
      const editor = dialog.locator('textarea').last();
      const yaml = await editor.inputValue();
      if (tpl.config?.image) {
        expect(yaml, `expected ${tpl.config.image} in compose YAML`).toContain(tpl.config.image);
      }
      // The new service's derived name comes from tpl.id (hyphens -> _);
      // blank is 'service' and can collide with the default, so only check
      // non-blank templates.
      if (tpl.id !== 'blank') {
        const derived = tpl.id.replace(/-/g, '_');
        expect(yaml).toContain(derived);
      }
    });
  }
});
