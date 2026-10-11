import { test, expect } from '@playwright/test';
import { getState, TEST_ADMIN } from '../helpers/auth.js';

// Creates the admin the rest of the suite uses. Setup input validation is a
// unit concern (ui Login.test.js, internal/api/auth_test.go).
test.describe('Initial Setup', () => {
  test('creates the first admin account through the setup form', async ({ page }) => {
    const state = getState();
    const before = await page.request.get(`${state.baseURL}/api/setup/status`);
    expect(await before.json()).toEqual({ needs_setup: true });

    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#displayName').fill(TEST_ADMIN.displayName);
    await page.locator('#email').fill(TEST_ADMIN.email);
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill(TEST_ADMIN.password);
    await page.locator('#confirmPassword').fill(TEST_ADMIN.password);
    const setupRes = page.waitForResponse((r) => r.url().endsWith('/api/setup') && r.request().method() === 'POST');
    await page.getByRole('button', { name: 'Create Account' }).click();
    expect((await setupRes).status()).toBe(201);
    await page.waitForSelector('aside:not([data-testid="activity-sidebar"])', { timeout: 15_000 });

    const me = await page.request.get(`${state.baseURL}/api/me`);
    expect(me.status()).toBe(200);
    const body = await me.json();
    expect(body.username).toBe(TEST_ADMIN.username);
    expect(body.role).toBe('super_admin');
    expect(body.display_name).toBe(TEST_ADMIN.displayName);
    expect(body.email).toBe(TEST_ADMIN.email);

    const after = await page.request.get(`${state.baseURL}/api/setup/status`);
    expect(await after.json()).toEqual({ needs_setup: false });
  });
});
