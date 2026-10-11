import { readFileSync } from 'fs';
import { join } from 'path';

const STATE_FILE = join(import.meta.dirname, '..', '.e2e-state.json');

export function getState() {
  return JSON.parse(readFileSync(STATE_FILE, 'utf-8'));
}

export const TEST_ADMIN = {
  username: 'e2eadmin',
  password: 'E2eTestPass123!',
  displayName: 'E2E Admin',
  email: 'e2e@test.local',
};

// loginViaUI signs in through the login form. Use it where the form itself
// is under test; everything else uses login(), which is faster.
export async function loginViaUI(page, username, password) {
  const state = getState();
  await page.goto(`${state.baseURL}/#/login`);
  // Wait for login form to finish loading (setupStatus check)
  await page.waitForSelector('#username', { timeout: 10_000 });
  await page.locator('#username').fill(username || TEST_ADMIN.username);
  await page.locator('#password').fill(password || TEST_ADMIN.password);
  // Wait for Sign In button (not Create Account - setupStatus must resolve)
  await page.getByRole('button', { name: 'Sign In' }).waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Sign In' }).click();
  // Wait for sidebar <aside> to appear (reliable post-login indicator)
  await page.waitForSelector('aside:not([data-testid="activity-sidebar"])', { timeout: 15_000 });
}

// login signs in through the API (page.request shares the browser context's
// cookie jar), then opens the dashboard.
export async function login(page, username, password) {
  const state = getState();
  const res = await page.request.post(`${state.baseURL}/api/auth/login`, {
    data: { username: username || TEST_ADMIN.username, password: password || TEST_ADMIN.password },
  });
  if (!res.ok()) throw new Error(`login failed: ${res.status()} ${await res.text()}`);
  await page.goto(`${state.baseURL}/#/`);
  await page.waitForSelector('aside:not([data-testid="activity-sidebar"])', { timeout: 15_000 });
}

export async function loginAsAdmin(page) {
  await login(page, TEST_ADMIN.username, TEST_ADMIN.password);
}

export async function logout(page) {
  await page.goto(`${getState().baseURL}/#/profile`);
  await page.getByText('Log out').click();
  // Wait for login page to appear
  await page.waitForSelector('#username', { timeout: 5_000 });
}
