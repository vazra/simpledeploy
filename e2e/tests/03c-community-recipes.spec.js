// E2E: Community recipes browse + import flow.
//
// Runs against an isolated server whose recipes_index_url points at a local
// fixture catalog (helpers/recipes.js), so the spec never depends on the
// live catalog. It imports a recipe into the wizard editor; it does not deploy.

import { test, expect } from '@playwright/test';
import { startServer, stopServer, getBinaryPath } from '../helpers/server.js';
import { apiRequestAt } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { startRecipesCatalog, FIXTURE_RECIPE } from '../helpers/recipes.js';

test.describe('community recipes', () => {
  let catalog = null;
  let server = null;

  test.beforeAll(async () => {
    catalog = await startRecipesCatalog();
    server = await startServer(getBinaryPath(), { recipesIndexURL: catalog.indexURL });
    const setup = await apiRequestAt(server.baseURL, 'POST', '/api/setup', {
      username: TEST_ADMIN.username,
      password: TEST_ADMIN.password,
      display_name: TEST_ADMIN.displayName,
      email: TEST_ADMIN.email,
    });
    expect(setup.status).toBe(201);
  });

  test.afterAll(async () => {
    stopServer(server);
    if (catalog) await catalog.stop();
  });

  test('browse, view detail, import a recipe into deploy wizard', async ({ page }) => {
    const login = await page.request.post(`${server.baseURL}/api/auth/login`, {
      data: { username: TEST_ADMIN.username, password: TEST_ADMIN.password },
    });
    expect(login.status()).toBe(200);
    await page.goto(`${server.baseURL}/#/`);

    await page.locator('main').getByRole('button', { name: 'Deploy App' }).first().click();
    const wizard = page.getByRole('dialog');
    await expect(wizard).toBeVisible();
    await wizard.getByRole('button', { name: /^browse templates$/i }).click();
    await wizard.getByRole('button', { name: /^browse community recipes$/i }).click();

    const browser = page.getByRole('dialog', { name: /browse community recipes/i });
    await expect(browser).toBeVisible();
    await browser.getByRole('button', { name: new RegExp(FIXTURE_RECIPE.name) }).click();

    // README comes from the catalog through the server's file proxy.
    await expect(browser.locator('pre')).toContainText('Fixture README marker e2e-readme-7f3a.');
    await browser.getByRole('button', { name: /^use recipe$/i }).click();
    await expect(browser).toBeHidden();

    await expect(wizard.locator('textarea').last()).toHaveValue(FIXTURE_RECIPE.compose);
    await expect(wizard.getByPlaceholder('my-app')).toHaveValue(FIXTURE_RECIPE.id);
    await expect(wizard.getByText('Valid compose file')).toBeVisible({ timeout: 10_000 });

    // Using a recipe records a compose pull for the popularity report.
    const pop = await page.request.get(`${server.baseURL}/api/recipes/community/popularity`);
    expect(pop.status()).toBe(200);
    expect((await pop.json()).recipes).toEqual([{ recipe_id: FIXTURE_RECIPE.id, count: 1 }]);
  });
});
