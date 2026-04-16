import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState } from '../helpers/auth.js';

test.describe('Endpoints & Access', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-nginx`);
    await page.getByRole('button', { name: 'Settings' }).click();
  });

  test('shows current endpoints', async ({ page }) => {
    await expect(page.getByText('nginx-test.local').first()).toBeVisible({ timeout: 10_000 });
  });

  test('shows TLS mode', async ({ page }) => {
    await expect(page.getByText(/No TLS|Auto TLS|Local CA|Custom TLS/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('IP allowlist section visible', async ({ page }) => {
    // Click Advanced collapsible section via evaluate to avoid click interception
    await page.evaluate(() => {
      const headings = document.querySelectorAll('h3');
      for (const h of headings) {
        if (h.textContent.trim() === 'Advanced') {
          h.closest('button').click();
          break;
        }
      }
    });
    await expect(page.locator('#allowlist-input')).toBeVisible({ timeout: 10_000 });
  });
});
