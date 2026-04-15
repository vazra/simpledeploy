import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState } from '../helpers/auth.js';

const FIXTURES = join(import.meta.dirname, '..', 'fixtures');

function readFixture(name) {
  return readFileSync(join(FIXTURES, name), 'utf-8');
}

async function deployApp(page, appName, composeContent) {
  await page.getByRole('button', { name: 'Deploy App' }).first().click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.getByPlaceholder('my-app').fill(appName);
  await page.getByRole('button', { name: 'YAML' }).click();
  const editor = page.locator('textarea').last();
  await editor.fill(composeContent);
  await expect(page.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Next' }).click();
  await page.getByRole('button', { name: 'Deploy' }).click();
  await expect(page.getByText(/deployed|complete/i)).toBeVisible({ timeout: 180_000 });
  const viewAppBtn = page.getByRole('button', { name: 'View App' });
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
    await expect(page.getByRole('dialog')).toBeVisible();
    await page.getByPlaceholder('my-app').fill('bad-app');
    await page.getByRole('button', { name: 'YAML' }).click();
    const editor = page.locator('textarea').last();
    await editor.fill('this is not: valid: yaml: [');
    await expect(page.getByText(/error|invalid/i)).toBeVisible({ timeout: 10_000 });
  });

  test('reject duplicate app name', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/`);
    const compose = readFixture('compose-nginx.yml');
    await page.getByRole('button', { name: 'Deploy App' }).first().click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await page.getByPlaceholder('my-app').fill('e2e-nginx');
    await page.getByRole('button', { name: 'YAML' }).click();
    const editor = page.locator('textarea').last();
    await editor.fill(compose);
    await expect(page.getByText(/valid compose/i)).toBeVisible({ timeout: 10_000 });
    await page.getByRole('button', { name: 'Next' }).click();
    await page.getByRole('button', { name: 'Deploy' }).click();
    await expect(page.getByText(/exists|duplicate|already/i)).toBeVisible({ timeout: 15_000 });
  });
});
