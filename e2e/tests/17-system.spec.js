import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, getSessionCookie } from '../helpers/api.js';
import { getAppId, sqliteExec } from '../helpers/db.js';
import { readFileSync, writeFileSync, rmSync, mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import { execFileSync } from 'child_process';

const SQLITE_MAGIC = 'SQLite format 3\x00';

test.describe('System Administration', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/system`);
  });

  test('overview shows live system info and the Native deployment', async ({ page }) => {
    const info = await apiRequest('GET', '/api/system/info');
    expect(info.status).toBe(200);
    await expect(page.locator('a[href="#/docker"]').getByText('Native', { exact: true })).toBeVisible();

    const main = page.locator('main');
    await expect(main.getByRole('heading', { name: 'Deployment', exact: true })).toBeVisible();
    await expect(main.getByText('Native Binary', { exact: true })).toBeVisible();
    await expect(main.getByText(/Values reflect/)).toHaveCount(0);

    const value = (label) => main.locator(`div:has(> div:text-is("${label}")) > div:nth-child(2)`);
    await expect(value('Uptime')).toHaveText(/\d/);
    await expect(value('CPU Cores')).toHaveText(String(info.data.resources.cpu_count));
    await expect(value('Path')).toHaveText(info.data.database.path);
  });

  test('maintenance: VACUUM succeeds and Download Now saves a SQLite file', async ({ page }) => {
    await page.locator('button').filter({ hasText: 'Maintenance' }).click();
    await page.getByRole('button', { name: 'Run VACUUM' }).click();
    await expect(page.getByRole('alert').filter({ hasText: 'VACUUM completed' })).toBeVisible();

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByRole('button', { name: 'Download Now' }).click(),
    ]);
    expect(download.suggestedFilename()).toMatch(/^simpledeploy-\d{8}-\d{6}\.db$/);
    const buf = readFileSync(await download.path());
    expect(buf.subarray(0, 16).toString('binary')).toBe(SQLITE_MAGIC);
    await expect(page.getByRole('alert').filter({ hasText: 'Backup downloaded' })).toBeVisible();
  });

  test('logs tab renders process log lines', async ({ page }) => {
    await page.locator('button').filter({ hasText: /^Logs$/ }).click();
    const logs = page.locator('#logs-container');
    await expect(logs.locator('.font-mono > div').first()).toBeVisible();
    await expect(logs).toContainText(/[1-9]\d* entries/);
  });
});

test.describe('System DB Backup - Functional', () => {
  test.describe.configure({ mode: 'serial' });

  const tmpDir = mkdtempSync(join(tmpdir(), 'sd-db-backup-'));
  const SEED_LATENCY = 4242.5;
  let fullAppsCount = 0;

  async function downloadDB(compact) {
    const res = await fetch(`${getState().baseURL}/api/system/backup/download${compact ? '?compact=true' : ''}`, {
      method: 'POST',
      headers: { Cookie: getSessionCookie() },
    });
    expect(res.status).toBe(200);
    const buf = Buffer.from(await res.arrayBuffer());
    expect(buf.subarray(0, 16).toString('binary')).toBe(SQLITE_MAGIC);
    const dest = join(tmpDir, compact ? 'compact.db' : 'full.db');
    writeFileSync(dest, buf);
    return (sql) => execFileSync('sqlite3', [dest, sql], { encoding: 'utf-8' }).trim();
  }

  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    // Seed one row in each table compact mode strips, so neither check passes vacuously.
    const appId = getAppId('e2e-nginx');
    const now = Math.floor(Date.now() / 1000);
    sqliteExec(`INSERT INTO metrics (app_id, container_id, ts, tier, cpu_pct, mem_bytes, mem_limit) VALUES (${appId}, 'e2e-backup-seed', ${now}, '1d', 1, 1, 4);`);
    sqliteExec(`INSERT INTO request_metrics (app_id, ts, tier, count, max_latency) VALUES (${appId}, ${now}, '1d', 1, ${SEED_LATENCY});`);
  });

  test.afterAll(() => {
    sqliteExec(`DELETE FROM metrics WHERE container_id = 'e2e-backup-seed';`);
    sqliteExec(`DELETE FROM request_metrics WHERE max_latency = ${SEED_LATENCY};`);
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test('full backup is a VACUUM INTO snapshot with apps and metrics', async () => {
    const q = await downloadDB(false);
    expect(q('SELECT slug FROM apps ORDER BY slug').split('\n')).toEqual(
      expect.arrayContaining(['e2e-multi', 'e2e-nginx', 'e2e-postgres']),
    );
    expect(q(`SELECT COUNT(*) FROM metrics WHERE container_id = 'e2e-backup-seed'`)).toBe('1');
    expect(q(`SELECT COUNT(*) FROM request_metrics WHERE max_latency = ${SEED_LATENCY}`)).toBe('1');
    fullAppsCount = Number(q('SELECT COUNT(*) FROM apps'));
  });

  test('compact backup strips metrics and request_metrics but keeps apps', async () => {
    const q = await downloadDB(true);
    expect(Number(q('SELECT COUNT(*) FROM apps'))).toBe(fullAppsCount);
    expect(q('SELECT COUNT(*) FROM metrics')).toBe('0');
    expect(q('SELECT COUNT(*) FROM request_metrics')).toBe('0');
  });
});
