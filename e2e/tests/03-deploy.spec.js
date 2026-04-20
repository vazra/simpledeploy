import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState } from '../helpers/auth.js';
import {
  findServiceContainer,
  containerRunning,
  containerImage,
  dockerInspect,
  listAppContainers,
  dockerExec,
} from '../helpers/docker.js';
import { fetchViaProxy } from '../helpers/proxy.js';
import { apiRequest, apiLogin } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';

const FIXTURES = join(import.meta.dirname, '..', 'fixtures');

function readFixture(name) {
  return readFileSync(join(FIXTURES, name), 'utf-8');
}

async function deployApp(page, appName, composeContent) {
  await page.getByRole('button', { name: 'Deploy App' }).first().click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  // Wizard opens on step 0 (Start chooser). Pick "Build it yourself" -> step 1 visual blank.
  await dialog.getByRole('button', { name: /build it yourself/i }).click();
  await dialog.getByPlaceholder('my-app').fill(appName);
  await dialog.getByRole('button', { name: 'YAML' }).click();
  const editor = dialog.locator('textarea').last();
  await editor.fill(composeContent);
  await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Next' }).click();
  await dialog.getByRole('button', { name: 'Deploy' }).click();
  await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });
  const viewAppBtn = dialog.getByRole('button', { name: 'View App' });
  if (await viewAppBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await viewAppBtn.click();
  }
}

test.describe('Deploy Apps', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('deploy nginx app', async ({ page }) => {
    const compose = readFixture('compose-nginx.yml');
    await deployApp(page, 'e2e-nginx', compose);
  });

  test('deploy multi-service app', async ({ page }) => {
    const compose = readFixture('compose-multi.yml');
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await deployApp(page, 'e2e-multi', compose);
  });

  test('deploy postgres app', async ({ page }) => {
    const compose = readFixture('compose-postgres.yml');
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await deployApp(page, 'e2e-postgres', compose);
  });

  test('reject invalid YAML', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /build it yourself/i }).click();
    await dialog.getByPlaceholder('my-app').fill('bad-app');
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    await editor.fill('this is not: valid: yaml: [');
    await expect(dialog.getByText(/failed|error|invalid/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('deploying same template twice auto-suggests name-2', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);

    // First deploy: pick nginx-static template, name it collide-tpl.
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    let dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /browse templates/i }).click();
    await dialog.getByRole('button', { name: /use template nginx static site/i }).click();
    // On vars step: default access mode is quick-test with prefilled host. Apply.
    await dialog.getByRole('button', { name: /apply/i }).click();
    // Step 1: set app name.
    const nameInput = dialog.getByPlaceholder('my-app');
    await expect(nameInput).toBeVisible();
    await nameInput.fill('collide-tpl');
    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();
    await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });
    // Close wizard via View App (matches the deployApp helper).
    const viewApp = dialog.getByRole('button', { name: 'View App' });
    if (await viewApp.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await viewApp.click();
    } else {
      await page.keyboard.press('Escape');
    }
    await expect(dialog).toBeHidden({ timeout: 10_000 });
    await page.goto(`${state.baseURL}/#/`);

    // Second deploy: same template, same name -> expect name-taken modal.
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /browse templates/i }).click();
    await dialog.getByRole('button', { name: /use template nginx static site/i }).click();
    await dialog.getByRole('button', { name: /apply/i }).click();
    const nameInput2 = dialog.getByPlaceholder('my-app');
    await nameInput2.fill('collide-tpl');
    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();

    // Expect the name-taken modal with pre-filled suggestion "collide-tpl-2".
    const modal = page.getByTestId('name-taken-modal');
    await expect(modal).toBeVisible({ timeout: 15_000 });
    const suggestionInput = page.getByTestId('name-taken-input');
    await expect(suggestionInput).toHaveValue('collide-tpl-2');
    await modal.getByRole('button', { name: 'Deploy' }).click();

    await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });

    // Both apps should exist.
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const list = await apiRequest('GET', '/api/apps');
    const names = (list.data || []).map((a) => a.Name || a.name);
    expect(names).toContain('collide-tpl');
    expect(names).toContain('collide-tpl-2');
  });

  test('manual deploy with existing name shows inline error and does not overwrite', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);

    // Capture existing compose hash for e2e-nginx before the attempt.
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const before = await apiRequest('GET', '/api/apps/e2e-nginx');
    expect(before.ok).toBe(true);
    const beforeHash = before.data && (before.data.ComposeHash || before.data.compose_hash);

    // Attempt a manual deploy reusing e2e-nginx name with a different compose.
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /build it yourself/i }).click();
    await dialog.getByPlaceholder('my-app').fill('e2e-nginx');
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    // A valid but different compose (distinct image tag) so hash would change if it overwrote.
    await editor.fill(
      "services:\n  web:\n    image: nginx:1.25-alpine\n    ports:\n      - '8080:80'\n"
    );
    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();

    // Expect inline name error on step 1 (no overwrite path in manual source).
    await expect(dialog.getByText(/already exists/i).first()).toBeVisible({ timeout: 10_000 });
    // No template suggestion modal should appear for manual source.
    await expect(page.getByTestId('name-taken-modal')).toHaveCount(0);

    // Verify the original e2e-nginx compose hash is unchanged.
    const after = await apiRequest('GET', '/api/apps/e2e-nginx');
    const afterHash = after.data && (after.data.ComposeHash || after.data.compose_hash);
    expect(afterHash).toBe(beforeHash);
  });
});

test.describe.configure({ mode: 'serial' });
test.describe('Deploy - Functional', () => {
  test('nginx container is running with correct image and labels', async () => {
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name, 'expected to find e2e-nginx web container').toBeTruthy();
    expect(containerRunning(name)).toBe(true);
    const img = containerImage(name);
    // Tolerate the GHCR image mirror (E2E_USE_MIRROR=1 rewrites image
    // refs server-side), so match on the trailing repo:tag instead of
    // anchoring at the start.
    expect(img).toMatch(/(^|\/)nginx:/);
    const info = dockerInspect(name);
    expect(info.Config.Labels['com.docker.compose.project']).toBe('simpledeploy-e2e-nginx');
    expect(info.Config.Labels['com.docker.compose.service']).toBe('web');
  });

  test('proxy routes HTTP traffic for nginx-test.local', async () => {
    const res = await fetchViaProxy('nginx-test.local', '/');
    expect(res.status).toBe(200);
    const body = await res.text();
    expect(body).toContain('Welcome to nginx');

    const missing = await fetchViaProxy('nonexistent.local', '/');
    const missingBody = await missing.text();
    // Caddy returns 200 with empty body when no route matches; ensure no nginx content.
    expect(missingBody).not.toContain('Welcome to nginx');
  });

  test('multi-service deploy runs both services and routes traffic', async () => {
    const containers = listAppContainers('e2e-multi');
    expect(containers.length).toBe(2);
    for (const c of containers) {
      expect(containerRunning(c)).toBe(true);
    }
    const res = await fetchViaProxy('multi-test.local', '/');
    expect(res.status).toBe(200);
    const body = await res.text();
    expect(body).toContain('Welcome to nginx');

    const redis = containers.find((n) => n.includes('cache')) || findServiceContainer('e2e-multi', 'cache');
    expect(redis, 'expected to find redis/cache container').toBeTruthy();
    const out = dockerExec(redis, 'redis-cli PING').trim();
    expect(out).toBe('PONG');
  });
});
