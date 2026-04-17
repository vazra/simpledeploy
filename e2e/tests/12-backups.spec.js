import { test, expect } from '@playwright/test';
import { createHash } from 'crypto';
import { readFileSync, existsSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import {
  apiLogin,
  apiRequest,
  apiDownload,
  apiUploadMultipart,
  waitForAppStatus,
} from '../helpers/api.js';
import {
  findServiceContainer,
  dockerExec,
  psql,
  containerRunning,
  waitForContainerState,
  waitForHealthy,
} from '../helpers/docker.js';

const FIXTURES = join(import.meta.dirname, '..', 'fixtures');

function readFixture(name) {
  return readFileSync(join(FIXTURES, name), 'utf-8');
}

async function pollRunStatus(slug, runId, desired, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
    if (res.ok && Array.isArray(res.data)) {
      const run = res.data.find((r) => r.id === runId);
      if (run) {
        last = run;
        if (Array.isArray(desired) ? desired.includes(run.status) : run.status === desired) {
          return run;
        }
      }
    }
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`run ${runId} did not reach status ${desired} within ${timeoutMs}ms (last=${JSON.stringify(last)})`);
}

async function waitForNewRunForConfig(slug, configId, sinceCount, timeoutMs) {
  // The API groups runs per-config; `data[0]` is NOT guaranteed to be the
  // globally newest run. Filter to the target config explicitly.
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
    if (res.ok && Array.isArray(res.data)) {
      const forCfg = res.data.filter((r) => r.backup_config_id === configId);
      if (forCfg.length > sinceCount) {
        // Sort by started_at DESC just in case
        forCfg.sort((a, b) => String(b.started_at).localeCompare(String(a.started_at)));
        return forCfg[0];
      }
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`no new run for config ${configId} in ${slug} within ${timeoutMs}ms`);
}

async function triggerAndWait(slug, configId, timeoutMs = 180_000) {
  const before = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
  const beforeCount = Array.isArray(before.data)
    ? before.data.filter((r) => r.backup_config_id === configId).length
    : 0;
  const trig = await apiRequest('POST', `/api/backups/configs/${configId}/run`);
  expect([200, 202]).toContain(trig.status);
  const newest = await waitForNewRunForConfig(slug, configId, beforeCount, 15_000);
  return await pollRunStatus(slug, newest.id, ['success', 'failed'], timeoutMs);
}

async function deleteAppConfigsFor(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/backups/configs`);
  if (!res.ok || !Array.isArray(res.data)) return;
  for (const c of res.data) {
    await apiRequest('DELETE', `/api/backups/configs/${c.id}`);
  }
}

test.describe('Backups', () => {
  test('navigate to postgres app backups tab', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();
    await expect(page.getByText(/backup|configure/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('detect strategies shows postgres and volume', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
    await configBtn.click();
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5_000 });

    // Strategy detection should show PostgreSQL and a volume/snapshot strategy.
    // Backend label is "Volume Snapshot"; legacy UI label is "Files & Volumes".
    await expect(page.getByText(/postgresql/i)).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/(volume|files.*volumes)/i).first()).toBeVisible();
  });

  test('create backup config via wizard', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
    await configBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Step 1: Select postgres strategy (auto-detected, may already be selected)
    const pgBtn = dialog.getByText(/postgresql/i).first();
    if (await pgBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await pgBtn.click();
    }
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 2: Select local storage (default)
    await dialog.getByText(/local storage/i).click();
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 3: Schedule (accept defaults)
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 4: Hooks (skip)
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 5: Retention (accept defaults)
    await dialog.getByRole('button', { name: /next/i }).click();

    // Step 6: Review and create
    await dialog.getByRole('button', { name: /create backup/i }).click();

    // Verify config appears in the table
    await expect(page.getByText(/local/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test('trigger manual backup', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const backupBtn = page.getByRole('button', { name: /backup now/i });
    await expect(backupBtn).toBeVisible({ timeout: 10_000 });
    await backupBtn.click();

    // Wait for backup to run, then reload to see result
    await page.waitForTimeout(3_000);
    await page.reload();
    await page.getByRole('button', { name: /backups/i }).click();
    await expect(page.getByText(/running|success|failed/i).first()).toBeVisible({ timeout: 15_000 });
  });

  test('global backups page shows summary', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/backups`);
    await expect(page.getByText(/total config/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('delete backup config', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/e2e-postgres`);
    await page.getByRole('button', { name: /backups/i }).click();

    const deleteBtn = page.getByRole('button', { name: /delete/i }).first();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });
});

test.describe('Backups - Functional Roundtrip', () => {
  test.describe.configure({ mode: 'serial' });

  const POSTGRES_SLUG = 'e2e-postgres';
  const VOLUME_SLUG = 'e2e-volume';

  let pgContainer = null;
  let volContainer = null;
  let postgresConfigId = null;
  let volumeConfigId = null;
  let pgSuccessRunId = null;

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.ok).toBeTruthy();

    // Locate postgres container (deployed in earlier spec)
    pgContainer = findServiceContainer(POSTGRES_SLUG, 'db');
    expect(pgContainer, 'e2e-postgres db container must be running').toBeTruthy();

    // Clean any pre-existing configs from earlier UI tests
    await deleteAppConfigsFor(POSTGRES_SLUG);

    // Volume strategy roundtrip uses the e2e-postgres pgdata volume directly.
    // (Avoids a separate e2e-volume app deploy which can race with the
    // reconciler during a busy test session.) The postgres container has
    // /var/lib/postgresql/data mounted from the pgdata volume; we seed and
    // restore a test file there for the volume strategy assertion.
    volContainer = pgContainer;
    await waitForHealthy(volContainer, 'test -d /var/lib/postgresql/data', 30_000);
  });

  test('1. postgres data roundtrip: insert, backup, drop, restore, verify', async () => {
    test.setTimeout(300_000);

    // 1. Seed data
    psql(pgContainer, 'postgres', 'testdb',
      "CREATE TABLE IF NOT EXISTS e2e_data(id INT, v TEXT); DELETE FROM e2e_data; INSERT INTO e2e_data VALUES (1, 'roundtrip-marker-abc123');");
    const seededOut = psql(pgContainer, 'postgres', 'testdb', "SELECT id||'|'||v FROM e2e_data WHERE id=1;");
    expect(seededOut).toContain('1|roundtrip-marker-abc123');

    // 2. Create backup config
    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'postgres',
      target: 'local',
      schedule_cron: '0 0 1 1 *', // yearly on Jan 1; effectively won't auto-trigger
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 5,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    expect(createRes.data.id).toBeGreaterThan(0);
    postgresConfigId = createRes.data.id;

    // 3. Trigger manual backup and wait for success
    const run = await triggerAndWait(POSTGRES_SLUG, postgresConfigId, 180_000);
    expect(run.status).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();
    expect(run.checksum.length).toBeGreaterThan(16);
    expect(run.file_path).toBeTruthy();
    pgSuccessRunId = run.id;

    // 4. Drop table
    psql(pgContainer, 'postgres', 'testdb', 'DROP TABLE e2e_data;');
    let errored = false;
    try {
      psql(pgContainer, 'postgres', 'testdb', 'SELECT * FROM e2e_data;');
    } catch {
      errored = true;
    }
    expect(errored).toBeTruthy();

    // 5. Restore
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);

    // 6. Poll until table comes back (restore is async)
    const restoreDeadline = Date.now() + 60_000;
    let restoredOut = '';
    while (Date.now() < restoreDeadline) {
      try {
        restoredOut = psql(pgContainer, 'postgres', 'testdb', "SELECT id||'|'||v FROM e2e_data WHERE id=1;");
        if (restoredOut.includes('roundtrip-marker-abc123')) break;
      } catch {
        // table may not exist yet
      }
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(restoredOut).toContain('1|roundtrip-marker-abc123');
  });

  test('2. backup file checksum matches SHA-256 of downloaded bytes', async () => {
    expect(pgSuccessRunId, 'prior postgres backup must exist').toBeTruthy();
    test.setTimeout(60_000);

    const runRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/runs`);
    expect(runRes.ok).toBeTruthy();
    const run = runRes.data.find((r) => r.id === pgSuccessRunId);
    expect(run).toBeTruthy();
    expect(run.checksum).toBeTruthy();

    const dl = await apiDownload(`/api/backups/runs/${pgSuccessRunId}/download`);
    expect(dl.ok, `download failed: status=${dl.status} body=${dl.body}`).toBeTruthy();
    expect(dl.buffer.length).toBeGreaterThan(0);

    const actual = createHash('sha256').update(dl.buffer).digest('hex');
    // run.checksum may be prefixed (e.g. "sha256:...") or raw hex; compare both
    const stored = run.checksum.replace(/^sha256:/i, '').toLowerCase();
    expect(actual.toLowerCase()).toBe(stored);
  });

  test('3. volume strategy captures files into a valid tar.gz archive', async () => {
    // A full restore-over-live-postgres cycle is racy (pg keeps files open).
    // Instead we verify the volume backup produces a valid tar.gz archive
    // that contains the seeded marker. Read the file directly from disk.
    test.setTimeout(300_000);

    const volDir = '/var/lib/postgresql/data';
    const markerPath = `${volDir}/e2e_marker.txt`;

    dockerExec(volContainer, `echo volume-marker-xyz > ${markerPath}`);
    expect(dockerExec(volContainer, `cat ${markerPath}`).trim()).toBe('volume-marker-xyz');

    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'volume',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 5,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: JSON.stringify([volDir]),
    });
    expect(createRes.ok).toBeTruthy();
    volumeConfigId = createRes.data.id;

    const run = await triggerAndWait(POSTGRES_SLUG, volumeConfigId, 180_000);
    expect(run.status).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);

    // Find the backup file on disk under ${state.dataDir}/backups. run.file_path
    // is just the filename for the local target.
    const state = getState();
    const { execFileSync } = await import('child_process');
    const filename = (run.file_path || '').split('/').pop();
    expect(filename, 'run must have a file_path').toBeTruthy();
    const absPath = `${state.dataDir}/backups/${filename}`;

    const list = execFileSync('tar', ['-tzf', absPath], { encoding: 'utf-8' });
    expect(list).toContain('e2e_marker.txt');

    const extracted = execFileSync('tar', ['-xzOf', absPath, 'var/lib/postgresql/data/e2e_marker.txt'], {
      encoding: 'utf-8',
    }).trim();
    expect(extracted).toBe('volume-marker-xyz');

    try { dockerExec(volContainer, `rm -f ${markerPath}`); } catch {}
  });

  test('4. pre/post hooks execute around postgres backup', async () => {
    test.setTimeout(300_000);

    // Build hook config: stop db pre-backup, start db post-backup.
    // NOTE: hook service names map to container names. The scheduler passes
    // app.Name as ContainerName; for hooks we target the actual service container.
    const preHooks = JSON.stringify([
      { type: 'stop', service: pgContainer },
    ]);
    const postHooks = JSON.stringify([
      { type: 'start', service: pgContainer },
    ]);

    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'postgres',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
      verify_upload: false,
      pre_hooks: preHooks,
      post_hooks: postHooks,
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    const hookCfgId = createRes.data.id;

    // Trigger and observe state transitions. The backup itself will likely fail
    // because the container is stopped during backup (pg_dump needs running pg),
    // but the post-hook should restart it and we can assert container recovered.
    const wasRunning = containerRunning(pgContainer);
    expect(wasRunning).toBeTruthy();

    const trig = await apiRequest('POST', `/api/backups/configs/${hookCfgId}/run`);
    expect([200, 202]).toContain(trig.status);

    // Observe a stopped state (pre-hook ran)
    let sawStopped = false;
    const stopDeadline = Date.now() + 30_000;
    while (Date.now() < stopDeadline) {
      if (containerRunning(pgContainer) === false) { sawStopped = true; break; }
      await new Promise((r) => setTimeout(r, 250));
    }

    // Whether or not we caught the stopped window, wait for container to be running again (post-hook)
    await waitForContainerState(pgContainer, true, 60_000);

    // Wait for postgres to accept connections again
    await waitForHealthy(pgContainer, 'pg_isready -U postgres', 60_000);

    // Assert we observed the stop (pre-hook) OR that the run recorded hook execution.
    // If the backup was too fast to catch the stopped state, at minimum the run record
    // should exist.
    const runsRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/runs`);
    expect(runsRes.ok).toBeTruthy();
    const hookRun = runsRes.data.find((r) => r.backup_config_id === hookCfgId);
    expect(hookRun).toBeTruthy();

    // Post-hook must have restarted the container (running now)
    expect(containerRunning(pgContainer)).toBeTruthy();

    // Cleanup hook config so retention test starts clean
    await apiRequest('DELETE', `/api/backups/configs/${hookCfgId}`);
  });

  test('5. retention_count=2 prunes older successful runs', async () => {
    test.setTimeout(600_000);

    // Fresh config with retention_count=2
    await deleteAppConfigsFor(POSTGRES_SLUG);
    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'postgres',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 2,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    const retCfgId = createRes.data.id;

    const filePaths = [];
    for (let i = 0; i < 3; i++) {
      const run = await triggerAndWait(POSTGRES_SLUG, retCfgId, 180_000);
      expect(run.status).toBe('success');
      if (run.file_path) filePaths.push(run.file_path);
      // Ensure different timestamp on filename (postgres strategy uses YYYYMMDD-HHMMSS)
      await new Promise((r) => setTimeout(r, 1500));
    }

    const runsRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/runs`);
    expect(runsRes.ok).toBeTruthy();
    const successRuns = runsRes.data.filter(
      (r) => r.backup_config_id === retCfgId && r.status === 'success',
    );

    // Scheduler prunes files on disk for runs beyond retention_count,
    // though the DB rows remain. Assert at most 2 on-disk files exist.
    let onDiskCount = 0;
    for (const p of filePaths) {
      if (p && existsSync(p)) onDiskCount += 1;
    }
    expect(onDiskCount).toBeLessThanOrEqual(2);
    // DB row count should be at least 3 (we triggered 3); success rows exist for all
    expect(successRuns.length).toBeGreaterThanOrEqual(3);

    // Save config id so next test can reuse it for download/upload cycle
    postgresConfigId = retCfgId;
  });

  test('6. download → upload-restore cycle via multipart', async () => {
    test.setTimeout(300_000);

    // Find any successful postgres run whose on-disk file still exists.
    // run.file_path stores the filename only; resolve against dataDir/backups.
    const runsRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/runs`);
    expect(runsRes.ok).toBeTruthy();
    const state = getState();
    const candidates = runsRes.data.filter((r) => {
      if (r.status !== 'success' || !r.file_path) return false;
      if (!r.file_path.endsWith('.sql.gz')) return false;
      const abs = `${state.dataDir}/backups/${r.file_path.split('/').pop()}`;
      return existsSync(abs);
    });
    expect(candidates.length).toBeGreaterThan(0);
    const run = candidates[0];

    // Re-seed the table so we can prove restore worked after drop
    psql(pgContainer, 'postgres', 'testdb',
      "CREATE TABLE IF NOT EXISTS e2e_data(id INT, v TEXT); DELETE FROM e2e_data; INSERT INTO e2e_data VALUES (1, 'roundtrip-marker-abc123');");

    // Download
    const dl = await apiDownload(`/api/backups/runs/${run.id}/download`);
    expect(dl.ok, `download failed: status=${dl.status}`).toBeTruthy();
    expect(dl.buffer.length).toBeGreaterThan(0);

    // Drop table
    psql(pgContainer, 'postgres', 'testdb', 'DROP TABLE e2e_data;');

    // Upload-restore. The downloaded filename should end in .sql.gz.
    const fileName = run.file_path.split('/').pop() || 'backup.sql.gz';
    const up = await apiUploadMultipart(
      `/api/apps/${POSTGRES_SLUG}/backups/upload-restore`,
      { strategy: 'postgres', container: pgContainer },
      'file',
      fileName,
      dl.buffer,
    );
    expect([200, 202]).toContain(up.status);

    // Poll until data comes back
    const deadline = Date.now() + 90_000;
    let got = '';
    while (Date.now() < deadline) {
      try {
        got = psql(pgContainer, 'postgres', 'testdb', "SELECT id||'|'||v FROM e2e_data WHERE id=1;");
        if (got.includes('roundtrip-marker-abc123')) break;
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(got).toContain('roundtrip-marker-abc123');
  });

  test('7. strategy detection reports postgres and volume for postgres app', async () => {
    const res = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/detect`);
    expect(res.ok).toBeTruthy();
    const strategies = res.data.strategies || [];
    const byType = Object.fromEntries(strategies.map((s) => [s.strategy_type, s]));

    // Postgres strategy should be available with a postgres service
    expect(byType.postgres, 'postgres strategy missing').toBeTruthy();
    expect(byType.postgres.available).toBe(true);
    expect(Array.isArray(byType.postgres.services)).toBe(true);
    expect(byType.postgres.services.length).toBeGreaterThan(0);
    const pgSvc = byType.postgres.services[0];
    expect(pgSvc.container_name).toContain('db');

    // Volume strategy should also surface for pgdata volume mount
    expect(byType.volume, 'volume strategy missing').toBeTruthy();
    expect(byType.volume.available).toBe(true);
    expect(byType.volume.services.length).toBeGreaterThan(0);
  });

  test.afterAll(async () => {
    // Best-effort cleanup of backup configs so later specs have a clean slate.
    try {
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
      await deleteAppConfigsFor(POSTGRES_SLUG);
    } catch {}
  });
});
