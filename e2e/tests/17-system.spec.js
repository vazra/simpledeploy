import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { readFileSync, writeFileSync, unlinkSync, mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import { execFileSync } from 'child_process';

test.describe('System Administration', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/system`);
  });

  test('system overview loads', async ({ page }) => {
    // Section heading "SimpleDeploy" in overview tab
    await expect(page.locator('h2').filter({ hasText: /SimpleDeploy/i }).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/uptime/i).first()).toBeVisible();
  });

  test('shows Native deployment badge in StatusBar and Deployment card on overview', async ({ page }) => {
    // beforeEach already navigated to /#/system and logged in
    await expect(page.locator('a[href="#/docker"]').getByText('Native', { exact: true })).toBeVisible({ timeout: 5_000 });

    const main = page.locator('main');
    await expect(main.getByRole('heading', { name: 'Deployment', exact: true })).toBeVisible({ timeout: 5_000 });
    await expect(main.getByText('Native Binary', { exact: true })).toBeVisible();
    await expect(main.getByText(/Values reflect/)).toHaveCount(0);
  });

  test('shows database info', async ({ page }) => {
    await expect(page.getByText(/database|sqlite/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('shows system resources', async ({ page }) => {
    await expect(page.getByText(/cpu|cores/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/ram|memory/i).first()).toBeVisible();
  });

  test('maintenance tab - vacuum database', async ({ page }) => {
    // Tab buttons are not role="button" by default in this UI; use text-based selector
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    const vacuumBtn = page.getByRole('button', { name: /Run VACUUM/i });
    await expect(vacuumBtn).toBeVisible({ timeout: 5_000 });
    await vacuumBtn.click();
    // Wait for toast or success indication
    await page.waitForTimeout(3_000);
  });

  test('maintenance tab - prune metrics', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    // "Prune Metrics" section has a select for tiers
    const tierSelect = page.locator('select').first();
    await expect(tierSelect).toBeVisible({ timeout: 5_000 });
  });

  test('maintenance tab - download database backup', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    const downloadBtn = page.getByRole('button', { name: /Download Now/i });
    await expect(downloadBtn).toBeVisible({ timeout: 5_000 });
  });

  test('audit log tab loads', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Audit Log' }).click();
    // Audit log section has heading "Audit Log" and table headers or empty state
    await expect(page.getByText(/No activity yet|Load more|Retention/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('logs tab loads', async ({ page }) => {
    await page.locator('button').filter({ hasText: /^Logs$/ }).click();
    // Logs tab has "Auto-scroll" checkbox label and "Refresh" button
    await expect(page.getByText(/Auto-scroll/i).first()).toBeVisible({ timeout: 5_000 });
  });
});

test.describe('System DB Backup - Functional', () => {
  test.describe.configure({ mode: 'serial' });

  const SQLITE_MAGIC = 'SQLite format 3\x00';
  const tmpDir = mkdtempSync(join(tmpdir(), 'sd-db-backup-'));
  const tmpFiles = [];

  function downloadDBBackup(compact) {
    const state = getState();
    const query = compact ? '?compact=true' : '';
    // Use curl to cleanly capture binary bytes and preserve auth cookie from apiRequest.
    // Simpler: use fetch with Cookie header relayed from apiRequest's session.
    // We re-login via fetch to capture cookie directly.
    return fetchWithAdminCookie('POST', `/api/system/backup/download${query}`);
  }

  async function fetchWithAdminCookie(method, path) {
    const state = getState();
    // Log in directly to grab cookie
    const loginRes = await fetch(`${state.baseURL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username: TEST_ADMIN.username,
        password: TEST_ADMIN.password,
      }),
    });
    expect(loginRes.ok).toBeTruthy();
    const setCookie = loginRes.headers.get('set-cookie') || '';
    const cookie = setCookie.split(';')[0];

    const res = await fetch(`${state.baseURL}${path}`, {
      method,
      headers: { Cookie: cookie },
    });
    return res;
  }

  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.afterAll(() => {
    for (const f of tmpFiles) {
      try { unlinkSync(f); } catch {}
    }
  });

  test('POST /api/system/backup/download returns valid sqlite bytes', async () => {
    const res = await downloadDBBackup(false);
    expect(res.status).toBe(200);
    const buf = Buffer.from(await res.arrayBuffer());
    expect(buf.length).toBeGreaterThan(0);

    // Magic header: "SQLite format 3\x00" = 16 bytes
    const magic = buf.slice(0, 16).toString('binary');
    expect(magic).toBe(SQLITE_MAGIC);

    // Write to disk and open with sqlite3 CLI
    const dest = join(tmpDir, 'backup-full.db');
    writeFileSync(dest, buf);
    tmpFiles.push(dest);

    const appsCount = Number(
      execFileSync('sqlite3', [dest, 'SELECT COUNT(*) FROM apps'], { encoding: 'utf-8' }).trim(),
    );
    // We expect at least 1 app in DB; the three test apps (e2e-nginx, e2e-multi,
    // e2e-postgres) should be present unless an earlier test already cleaned up.
    expect(appsCount).toBeGreaterThanOrEqual(1);
  });

  test('compact backup excludes metrics and request_metrics but preserves apps', async () => {
    const res = await downloadDBBackup(true);
    expect(res.status).toBe(200);
    const buf = Buffer.from(await res.arrayBuffer());
    expect(buf.length).toBeGreaterThan(0);

    const magic = buf.slice(0, 16).toString('binary');
    expect(magic).toBe(SQLITE_MAGIC);

    const dest = join(tmpDir, 'backup-compact.db');
    writeFileSync(dest, buf);
    tmpFiles.push(dest);

    // apps table should still have rows
    const appsCount = Number(
      execFileSync('sqlite3', [dest, 'SELECT COUNT(*) FROM apps'], { encoding: 'utf-8' }).trim(),
    );
    expect(appsCount).toBeGreaterThanOrEqual(1);

    // metrics table should be empty (stripped by compact mode)
    const metricsCount = Number(
      execFileSync('sqlite3', [dest, 'SELECT COUNT(*) FROM metrics'], { encoding: 'utf-8' }).trim(),
    );
    expect(metricsCount).toBe(0);

    // request_metrics table should also be empty
    const reqCount = Number(
      execFileSync('sqlite3', [dest, 'SELECT COUNT(*) FROM request_metrics'], { encoding: 'utf-8' }).trim(),
    );
    expect(reqCount).toBe(0);
  });

  test('GET /api/system/backup/runs returns an array', async () => {
    const res = await apiRequest('GET', '/api/system/backup/runs');
    expect(res.ok).toBeTruthy();
    expect(Array.isArray(res.data)).toBe(true);
    // Note: download endpoint does not record runs; only the cron job does.
    // Each run record (if present from a configured schedule) must have expected shape.
    for (const run of res.data) {
      expect(typeof run.id).toBe('number');
      expect(typeof run.file_path).toBe('string');
      expect(typeof run.size_bytes).toBe('number');
      expect(typeof run.status).toBe('string');
    }
  });
});
