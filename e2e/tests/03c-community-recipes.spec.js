// E2E: Community recipes browse + import flow.
//
// Hits the live community catalog at vazra.github.io/simpledeploy-recipes.
// The catalog must contain at least the seed recipe `nginx-static` for this
// spec to pass. Catalog source: https://github.com/vazra/simpledeploy-recipes
//
// This spec only imports a recipe into the wizard editor; it does not deploy
// (no DNS/domain dependency, fast).

import { test, expect } from '@playwright/test';
import { loginAsAdmin } from '../helpers/auth.js';

test.describe('community recipes', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('browse, view detail, import a recipe into deploy wizard', async ({ page }) => {
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog').first();
    await expect(dialog).toBeVisible();

    // Open the templates grid first (step 0 has Browse Templates / Build it yourself).
    await dialog.getByRole('button', { name: /^browse templates$/i }).click();

    // Open the community recipes browser from the template picker.
    await dialog.getByRole('button', { name: /^browse community recipes$/i }).click();

    // The browser opens as its own dialog. Scope subsequent queries to it.
    const browser = page.getByRole('dialog', { name: /browse community recipes/i });
    await expect(browser).toBeVisible({ timeout: 15_000 });

    // Live catalog must contain the nginx-static seed recipe.
    const card = browser.getByRole('button', { name: /nginx static site/i });
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();

    // Detail view shows README + Use Recipe button.
    await expect(browser.getByRole('button', { name: /^use recipe$/i })).toBeVisible({ timeout: 10_000 });

    // Import the recipe.
    await browser.getByRole('button', { name: /^use recipe$/i }).click();

    // Wizard advances to step 1 with YAML editor in 'yaml' mode.
    // The compose textarea should contain the recipe body.
    const editor = dialog.locator('textarea').last();
    await expect(editor).toBeVisible({ timeout: 10_000 });
    const yamlText = await editor.inputValue();
    expect(yamlText.toLowerCase()).toMatch(/image:\s*nginx/);
  });
});
