import { test, expect } from '@playwright/test';
import { getState, TEST_ADMIN, loginViaUI, logout } from '../helpers/auth.js';

// Other specs sign in through the API (helpers/auth.js login); the login
// form itself is only exercised here.
test.describe('Authentication', () => {
  test('login form signs in as the admin', async ({ page }) => {
    await loginViaUI(page);
    const me = await page.request.get(`${getState().baseURL}/api/me`);
    expect(me.status()).toBe(200);
    const body = await me.json();
    expect(body.username).toBe(TEST_ADMIN.username);
    expect(body.role).toBe('super_admin');
  });

  test('wrong password is rejected with 401 and stays on login', async ({ page }) => {
    const state = getState();
    await page.goto(`${state.baseURL}/#/login`);
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill('WrongPassword123!');
    const resP = page.waitForResponse('**/api/auth/login');
    await page.getByRole('button', { name: 'Sign In' }).click();
    expect((await resP).status()).toBe(401);
    await expect(page.getByText(/invalid credentials/i)).toBeVisible();
    await expect(page).toHaveURL(/login/);
    await expect(page.locator('aside:not([data-testid="activity-sidebar"])')).toHaveCount(0);
  });

  test('accessing protected route without session redirects to login', async ({ page }) => {
    await page.goto(`${getState().baseURL}/#/users`);
    await expect(page).toHaveURL(/login/, { timeout: 5_000 });
  });

  test('logout revokes the session server-side; logging in again works', async ({ page }) => {
    const state = getState();
    await loginViaUI(page);
    const old = (await page.context().cookies()).find((c) => c.name === 'session');
    expect(old, 'session cookie after login').toBeTruthy();

    await logout(page);
    await expect(page).toHaveURL(/login/);

    // The old cookie must be dead even if a client kept it.
    const stale = await fetch(`${state.baseURL}/api/me`, { headers: { Cookie: `session=${old.value}` } });
    expect(stale.status).toBe(401);

    await loginViaUI(page);
    expect((await page.request.get(`${state.baseURL}/api/me`)).status()).toBe(200);
  });
});
