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

const FIXTURES = join(import.meta.dirname, '..', 'fixtures');

function readFixture(name) {
  return readFileSync(join(FIXTURES, name), 'utf-8');
}

async function deployApp(page, appName, composeContent) {
  await page.getByRole('button', { name: 'Deploy App' }).first().click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await dialog.getByPlaceholder('my-app').fill(appName);
  await dialog.getByRole('button', { name: 'YAML' }).click();
  const editor = dialog.locator('textarea').last();
  await editor.fill(composeContent);
  await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Next' }).click();
  await dialog.getByRole('button', { name: 'Deploy' }).click();
  await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 180_000 });
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
    await dialog.getByPlaceholder('my-app').fill('bad-app');
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    await editor.fill('this is not: valid: yaml: [');
    await expect(dialog.getByText(/failed|error|invalid/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('redeploy existing app succeeds (update)', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    const compose = readFixture('compose-nginx.yml');
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByPlaceholder('my-app').fill('e2e-nginx');
    await dialog.getByRole('button', { name: 'YAML' }).click();
    const editor = dialog.locator('textarea').last();
    await editor.fill(compose);
    await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
    await dialog.getByRole('button', { name: 'Next' }).click();
    await dialog.getByRole('button', { name: 'Deploy' }).click();
    // Redeploying an existing app is an update, should succeed
    await expect(dialog.getByText('Deployed', { exact: true })).toBeVisible({ timeout: 180_000 });
  });
});

test.describe.configure({ mode: 'serial' });
test.describe('Deploy - Functional', () => {
  test('nginx container is running with correct image and labels', async () => {
    const name = findServiceContainer('e2e-nginx', 'web');
    expect(name, 'expected to find e2e-nginx web container').toBeTruthy();
    expect(containerRunning(name)).toBe(true);
    const img = containerImage(name);
    expect(img.startsWith('nginx:')).toBe(true);
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
