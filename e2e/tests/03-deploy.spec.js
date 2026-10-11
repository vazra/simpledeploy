import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, TEST_ADMIN } from '../helpers/auth.js';
import {
  findServiceContainer,
  containerRunning,
  containerImage,
  dockerInspect,
  listAppContainers,
  dockerExec,
} from '../helpers/docker.js';
import { fetchViaProxy, waitForProxyStatus } from '../helpers/proxy.js';
import { apiRequest, apiLogin, waitForAppStatus } from '../helpers/api.js';

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
  await dialog.locator('textarea').last().fill(composeContent);
  await expect(dialog.getByText('Valid compose file', { exact: true })).toBeVisible({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Next' }).click();
  await dialog.getByRole('button', { name: 'Deploy' }).click();
  await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });
  await dialog.getByRole('button', { name: 'View App' }).click();
  await expect(page).toHaveURL(new RegExp(`#/apps/${appName}$`));
  await expect(page.locator('h1')).toHaveText(appName);
  await waitForAppStatus(appName, 'running', 15_000);
}

test.describe('Deploy Apps', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('deploy nginx app', async ({ page }) => {
    await deployApp(page, 'e2e-nginx', readFixture('compose-nginx.yml'));
  });

  test('deploy multi-service app', async ({ page }) => {
    await deployApp(page, 'e2e-multi', readFixture('compose-multi.yml'));
  });

  test('deploy postgres app', async ({ page }) => {
    await deployApp(page, 'e2e-postgres', readFixture('compose-postgres.yml'));
  });

  test('invalid YAML fails server validation and blocks Next', async ({ page }) => {
    const bad = 'this is not: valid: yaml: [';
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /build it yourself/i }).click();
    await dialog.getByPlaceholder('my-app').fill('bad-app');
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const validated = page.waitForResponse((r) =>
      r.url().endsWith('/api/apps/validate-compose') &&
      r.request().postDataJSON()?.compose === Buffer.from(bad).toString('base64'));
    await dialog.locator('textarea').last().fill(bad);
    const res = await validated;
    expect(res.status()).toBe(200);
    expect((await res.json()).valid).toBe(false);
    await expect(dialog.getByText(/^Invalid compose file:/)).toBeVisible();
    await expect(dialog.getByText('Valid compose file', { exact: true })).toHaveCount(0);
    await expect(dialog.getByRole('button', { name: 'Next' })).toBeDisabled();
  });

  test('template deploy reusing an existing name suggests name-2 and keeps the original', async ({ page }) => {
    const before = await apiRequest('GET', '/api/apps/e2e-nginx');
    expect(before.status).toBe(200);
    const beforeHash = before.data.ComposeHash;
    expect(beforeHash).toBeTruthy();

    try {
      await page.getByRole('button', { name: 'Deploy App' }).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.getByRole('button', { name: /browse templates/i }).click();
      await dialog.getByRole('button', { name: /use template nginx static site/i }).click();
      await dialog.getByRole('button', { name: /^custom domain$/i }).click();
      await dialog.locator('#tpl-var-domain').fill('tpl-e2e.localhost');
      await dialog.getByRole('button', { name: /^apply/i }).click();
      await dialog.getByPlaceholder('my-app').fill('e2e-nginx');
      await expect(dialog.getByText('Valid compose file', { exact: true })).toBeVisible({ timeout: 10_000 });
      await dialog.getByRole('button', { name: 'Next' }).click();
      await dialog.getByRole('button', { name: 'Deploy' }).click();

      const modal = page.getByTestId('name-taken-modal');
      await expect(modal).toBeVisible({ timeout: 15_000 });
      await expect(page.getByTestId('name-taken-input')).toHaveValue('e2e-nginx-2');
      await modal.getByRole('button', { name: 'Deploy' }).click();
      await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 300_000 });

      // The template's domain variable reaches the deployed app.
      const copy = await waitForAppStatus('e2e-nginx-2', 'running', 15_000);
      expect(copy.Domain).toBe('tpl-e2e.localhost');
      const after = await apiRequest('GET', '/api/apps/e2e-nginx');
      expect(after.data.ComposeHash).toBe(beforeHash);
    } finally {
      // Leftover template endpoints break later plain-HTTP specs.
      await apiRequest('DELETE', '/api/apps/e2e-nginx-2');
    }
  });

  test('manual deploy with existing name shows inline error and does not overwrite', async ({ page }) => {
    const before = await apiRequest('GET', '/api/apps/e2e-nginx');
    expect(before.status).toBe(200);
    const beforeHash = before.data.ComposeHash;
    expect(beforeHash).toBeTruthy();

    // Attempt a manual deploy reusing e2e-nginx name with a different compose.
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: /build it yourself/i }).click();
    await dialog.getByPlaceholder('my-app').fill('e2e-nginx');
    await dialog.getByRole('button', { name: 'YAML' }).click();
    // A valid but different compose (distinct image tag) so hash would change if it overwrote.
    await dialog.locator('textarea').last().fill(
      "services:\n  web:\n    image: nginx:1.25-alpine\n    ports:\n      - '8080:80'\n"
    );
    await expect(dialog.getByText('Valid compose file', { exact: true })).toBeVisible({ timeout: 10_000 });
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();

    // Expect inline name error on step 1 (no overwrite path in manual source).
    await expect(dialog.getByText(/already exists/i)).toBeVisible({ timeout: 10_000 });
    await expect(page.getByTestId('name-taken-modal')).toHaveCount(0);

    const after = await apiRequest('GET', '/api/apps/e2e-nginx');
    expect(after.data.ComposeHash).toBe(beforeHash);
  });
});

test.describe('Deploy - Functional', () => {
  test('nginx container is running with correct image and labels', async () => {
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name, 'expected to find e2e-nginx web container').toBeTruthy();
    expect(containerRunning(name)).toBe(true);
    // Tolerate the GHCR image mirror (E2E_USE_MIRROR=1 rewrites image
    // refs server-side), so match on the trailing repo:tag.
    expect(containerImage(name)).toMatch(/(^|\/)nginx:alpine$/);
    const info = dockerInspect(name);
    expect(info.Config.Labels['com.docker.compose.project']).toBe('simpledeploy-e2e-nginx');
    expect(info.Config.Labels['com.docker.compose.service']).toBe('web');
  });

  test('proxy routes HTTP traffic for nginx-test.local', async () => {
    await waitForProxyStatus('nginx-test.local', 200, { timeoutMs: 30_000, bodyContains: 'Welcome to nginx' });
    // Caddy returns 200 with empty body when no route matches; ensure no nginx content.
    const missing = await fetchViaProxy('nonexistent.local', '/');
    expect(await missing.text()).not.toContain('Welcome to nginx');
  });

  test('multi-service deploy runs both services and routes traffic', async () => {
    const containers = listAppContainers('e2e-multi');
    expect(containers.length).toBe(2);
    for (const c of containers) {
      expect(containerRunning(c)).toBe(true);
    }
    await waitForProxyStatus('multi-test.local', 200, { timeoutMs: 30_000, bodyContains: 'Welcome to nginx' });

    const redis = findServiceContainer('e2e-multi', 'cache');
    expect(redis, 'expected to find redis/cache container').toBeTruthy();
    expect(dockerExec(redis, 'redis-cli PING').trim()).toBe('PONG');
  });
});
