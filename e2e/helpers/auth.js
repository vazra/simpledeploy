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

export async function login(page, username, password) {
  const state = getState();
  await page.goto(`${state.baseURL}/#/login`);
  await page.locator('#username').fill(username || TEST_ADMIN.username);
  await page.locator('#password').fill(password || TEST_ADMIN.password);
  await page.getByRole('button', { name: 'Sign In' }).click();
  // SPA uses hash routing; wait for sidebar/nav to confirm dashboard loaded
  await page.waitForSelector('aside nav, nav', { timeout: 10_000 });
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
