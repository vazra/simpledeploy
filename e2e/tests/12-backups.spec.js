import { test, expect } from '@playwright/test';
import { createHash } from 'crypto';
import { execFileSync } from 'child_process';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, apiDownload, apiUploadMultipart } from '../helpers/api.js';
import { findServiceContainer, dockerExec, dockerInspect, psql, waitForHealthy } from '../helpers/docker.js';
import {
  createBackupConfig,
  deleteAppConfigsFor,
  listRuns,
  localBackupExists,
  localBackupPath,
  pollRunStatus,
  readOr,
  triggerAndWait,
  waitForNewRunForConfig,
} from '../helpers/backups.js';

const POSTGRES_SLUG = 'e2e-postgres';

async function openBackupsTab(page) {
  await loginAsAdmin(page);
  await page.goto(`${getState().baseURL}/#/apps/${POSTGRES_SLUG}`);
  await page.getByRole('button', { name: /^backups$/i }).click();
}

async function listConfigs(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/backups/configs`);
  expect(res.status).toBe(200);
  return res.data;
}

test.describe('Backups', () => {
  test.describe.configure({ mode: 'serial' });

  let configId = null;

  test.beforeAll(async () => {
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.status).toBe(200);
    await deleteAppConfigsFor(POSTGRES_SLUG);
  });

  test('create backup config via wizard', async ({ page }) => {
    await openBackupsTab(page);
    await page.getByRole('button', { name: 'Configure Backup' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Step 1: strategy, detected from the compose file
    const pgCard = dialog.getByRole('button', { name: /^PostgreSQL/ });
    await expect(pgCard).toContainText('Detected');
    await pgCard.click();
    await dialog.getByRole('button', { name: 'Next' }).click();

    // Step 2: storage
    await dialog.getByRole('button', { name: /Local Storage/ }).click();
    await dialog.getByRole('button', { name: 'Next' }).click();

    // Steps 3-5: schedule, hooks, retention (defaults)
    for (let i = 0; i < 3; i++) {
      await dialog.getByRole('button', { name: 'Next' }).click();
    }

    // Step 6: review
    await dialog.getByRole('button', { name: 'Create Backup' }).click();
    await expect(dialog).toBeHidden();

    const configs = await listConfigs(POSTGRES_SLUG);
    expect(configs).toHaveLength(1);
    expect(configs[0]).toMatchObject({ strategy: 'postgres', target: 'local' });
    configId = configs[0].id;

    const row = page.locator('main table').first().getByRole('row').nth(1);
    await expect(row).toContainText('Database (PostgreSQL)');
    await expect(row).toContainText('Local storage');
  });

  test('trigger manual backup', async ({ page }) => {
    expect(configId, 'wizard config must exist').toBeTruthy();
    await openBackupsTab(page);

    const known = new Set((await listRuns(POSTGRES_SLUG)).map((r) => r.id));
    const trig = page.waitForResponse(
      (r) => r.url().endsWith(`/api/backups/configs/${configId}/run`) && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Backup Now' }).click();
    expect((await trig).status()).toBe(202);

    const run = await waitForNewRunForConfig(POSTGRES_SLUG, configId, known);
    const done = await pollRunStatus(POSTGRES_SLUG, run.id, ['success', 'failed'], 120_000);
    expect(done.status, `run error: ${done.error_msg}`).toBe('success');
  });

  test('global backups page shows summary', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/backups`);
    const stat = (label) =>
      page.locator('main').getByText(label, { exact: true }).locator('xpath=following-sibling::div[1]');
    await expect(stat('Total Configs')).toHaveText('1');
    expect(Number(await stat('24h Successful').textContent())).toBeGreaterThanOrEqual(1);
  });

  test('delete backup config', async ({ page }) => {
    expect(configId, 'wizard config must exist').toBeTruthy();
    await openBackupsTab(page);

    const configsTable = page.locator('main').getByRole('table').first();
    const row = configsTable.getByRole('row', { name: /Database \(PostgreSQL\)/ });
    await row.getByRole('button', { name: 'Delete' }).click();

    await expect(page.getByRole('heading', { name: 'No backups configured' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Backup Now' })).toHaveCount(0);
    expect(await listConfigs(POSTGRES_SLUG)).toEqual([]);
  });

  test('backups tab shows the finished run without a reload', async ({ page }) => {
    const cfgId = await createBackupConfig(POSTGRES_SLUG, { strategy: 'postgres' });
    try {
      await openBackupsTab(page);
      const known = new Set((await listRuns(POSTGRES_SLUG)).map((r) => r.id));
      await page.getByRole('button', { name: 'Backup Now' }).click();

      const run = await waitForNewRunForConfig(POSTGRES_SLUG, cfgId, known);
      const done = await pollRunStatus(POSTGRES_SLUG, run.id, ['success', 'failed'], 120_000);
      expect(done.status, `run error: ${done.error_msg}`).toBe('success');

      const history = page.locator('main').getByRole('table').nth(1);
      await expect(history.getByRole('row').nth(1)).toContainText('success');
    } finally {
      await apiRequest('DELETE', `/api/backups/configs/${cfgId}`);
    }
  });
});

test.describe('Backups - Functional Roundtrip', () => {
  test.describe.configure({ mode: 'serial' });

  const MARKER_SQL = "CREATE TABLE IF NOT EXISTS e2e_data(id INT, v TEXT); DELETE FROM e2e_data; INSERT INTO e2e_data VALUES (1, 'roundtrip-marker-abc123');";
  const SELECT_SQL = "SELECT id||'|'||v FROM e2e_data WHERE id=1;";

  let pgContainer = null;

  test.beforeAll(async () => {
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.status).toBe(200);

    pgContainer = findServiceContainer(POSTGRES_SLUG, 'db');
    expect(pgContainer, 'e2e-postgres db container must be running').toBeTruthy();

    await deleteAppConfigsFor(POSTGRES_SLUG);
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    await deleteAppConfigsFor(POSTGRES_SLUG);
  });

  test('1. postgres data roundtrip: insert, backup, drop, restore, verify', async () => {
    psql(pgContainer, 'postgres', 'testdb', MARKER_SQL);
    expect(psql(pgContainer, 'postgres', 'testdb', SELECT_SQL)).toBe('1|roundtrip-marker-abc123');

    const cfgId = await createBackupConfig(POSTGRES_SLUG, { strategy: 'postgres' });
    const run = await triggerAndWait(POSTGRES_SLUG, cfgId);
    expect(run.status, `run error: ${run.error_msg}`).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum.length).toBeGreaterThan(16);
    expect(localBackupExists(run), `backup file ${localBackupPath(run)}`).toBe(true);

    psql(pgContainer, 'postgres', 'testdb', 'DROP TABLE e2e_data;');
    expect(() => psql(pgContainer, 'postgres', 'testdb', 'SELECT * FROM e2e_data;')).toThrow(/does not exist/);

    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);
    await expect
      .poll(() => readOr(() => psql(pgContainer, 'postgres', 'testdb', SELECT_SQL)), { timeout: 60_000 })
      .toBe('1|roundtrip-marker-abc123');
  });

  test('2. volume strategy captures files into a valid tar.gz archive', async () => {
    // Restoring over a live postgres data dir is racy (pg keeps files open),
    // so verify the archive contents instead of a full roundtrip.
    const volDir = '/var/lib/postgresql/data';
    const markerPath = `${volDir}/e2e_marker.txt`;
    dockerExec(pgContainer, `echo volume-marker-xyz > ${markerPath}`);

    try {
      const cfgId = await createBackupConfig(POSTGRES_SLUG, { strategy: 'volume', paths: JSON.stringify([volDir]) });
      const run = await triggerAndWait(POSTGRES_SLUG, cfgId);
      expect(run.status, `run error: ${run.error_msg}`).toBe('success');
      expect(run.size_bytes).toBeGreaterThan(0);

      const absPath = localBackupPath(run);
      const list = execFileSync('tar', ['-tzf', absPath], { encoding: 'utf-8' });
      expect(list).toContain('var/lib/postgresql/data/e2e_marker.txt');
      const extracted = execFileSync('tar', ['-xzOf', absPath, 'var/lib/postgresql/data/e2e_marker.txt'], {
        encoding: 'utf-8',
      }).trim();
      expect(extracted).toBe('volume-marker-xyz');
    } finally {
      dockerExec(pgContainer, `rm -f ${markerPath}`);
    }
  });

  test('3. download, checksum, upload-restore cycle via multipart', async () => {
    const runs = await listRuns(POSTGRES_SLUG);
    const run = runs.find((r) => r.status === 'success' && r.file_path.endsWith('.sql.gz') && localBackupExists(r));
    expect(run, 'a postgres run from test 1 with its file on disk').toBeTruthy();

    psql(pgContainer, 'postgres', 'testdb', MARKER_SQL);

    const dl = await apiDownload(`/api/backups/runs/${run.id}/download`);
    expect(dl.status, `download failed: ${dl.body}`).toBe(200);
    const actual = createHash('sha256').update(dl.buffer).digest('hex');
    expect(actual).toBe(run.checksum.replace(/^sha256:/i, '').toLowerCase());

    psql(pgContainer, 'postgres', 'testdb', 'DROP TABLE e2e_data;');

    const up = await apiUploadMultipart(
      `/api/apps/${POSTGRES_SLUG}/backups/upload-restore`,
      { strategy: 'postgres', container: pgContainer },
      'file',
      run.file_path.split('/').pop(),
      dl.buffer,
    );
    expect(up.status, `upload-restore: ${JSON.stringify(up.data)}`).toBe(202);

    await expect
      .poll(() => readOr(() => psql(pgContainer, 'postgres', 'testdb', SELECT_SQL)), { timeout: 90_000 })
      .toBe('1|roundtrip-marker-abc123');
  });

  test('4. pre/post hooks run in the app container around the backup', async () => {
    // Hooks name compose services. The pre-hook leaves a marker; the post
    // hooks restart the db service (so its StartedAt changes) and then leave
    // a second marker, which needs the restarted container to be running.
    const tag = Date.now();
    const preMarker = `/tmp/e2e-pre-hook-${tag}`;
    const postMarker = `/tmp/e2e-post-hook-${tag}`;
    const startedBefore = dockerInspect(pgContainer).State.StartedAt;

    const cfgId = await createBackupConfig(POSTGRES_SLUG, {
      strategy: 'postgres',
      pre_hooks: JSON.stringify([{ type: 'exec', service: 'db', command: `touch ${preMarker}` }]),
      post_hooks: JSON.stringify([
        { type: 'stop', service: 'db' },
        { type: 'start', service: 'db' },
        { type: 'exec', service: 'db', command: `touch ${postMarker}` },
      ]),
    });

    try {
      const run = await triggerAndWait(POSTGRES_SLUG, cfgId);
      expect(run.status, `run error: ${run.error_msg}`).toBe('success');
      await waitForHealthy(pgContainer, 'pg_isready -U postgres', 60_000);

      expect(() => dockerExec(pgContainer, `test -f ${preMarker}`), 'pre-hook marker').not.toThrow();
      expect(() => dockerExec(pgContainer, `test -f ${postMarker}`), 'post-hook marker').not.toThrow();
      expect(dockerInspect(pgContainer).State.StartedAt, 'post-hooks restart db').not.toBe(startedBefore);
    } finally {
      await apiRequest('DELETE', `/api/backups/configs/${cfgId}`);
      await waitForHealthy(pgContainer, 'pg_isready -U postgres', 60_000);
    }
  });
});
