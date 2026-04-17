// E2E: non-postgres backup strategies (mysql, mongo, redis, sqlite).
// Each strategy runs in its own serial describe that deploys its own fixture app,
// exercises seed -> backup -> drop -> restore -> verify, then tears down.
//
// KNOWN BACKEND BUG (found during test authoring):
//   Scheduler.RunBackup / RunRestore build BackupOpts without ever populating
//   Credentials. The postgres strategy tolerates this (reads env inside the
//   container via `sh -c` script). mysql + mongo do NOT — they only consult
//   opts.Credentials for MYSQL_ROOT_PASSWORD / MONGO_INITDB_ROOT_* and will
//   therefore fail auth. The tests below will surface this: the backup run
//   status will come back "failed" with an auth error. See report.
//
// Fix direction: either
//   (a) read container env for MYSQL_ROOT_PASSWORD / MONGO_INITDB_ROOT_* via
//       `sh -c` inside the container (like postgres), or
//   (b) have Scheduler read env vars from the container (docker inspect) and
//       populate opts.Credentials before calling Backup/Restore.

import { test, expect } from '@playwright/test';
import { readFileSync, existsSync } from 'fs';
import { join } from 'path';
import { getState, TEST_ADMIN } from '../helpers/auth.js';
import {
  apiLogin,
  apiRequest,
  waitForAppStatus,
} from '../helpers/api.js';
import {
  findServiceContainer,
  waitForContainerState,
  waitForHealthy,
} from '../helpers/docker.js';
import {
  mysqlExec,
  mongoEval,
  redisCmd,
  sqlite3Eval,
} from '../helpers/dbclients.js';

const FIXTURES = join(import.meta.dirname, '..', 'fixtures');

function readFixture(name) {
  return readFileSync(join(FIXTURES, name), 'utf-8');
}

// --- shared helpers (kept local; near-duplicates of 12-backups.spec.js) ---

async function pollRunStatus(slug, runId, desired, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
    if (res.ok && Array.isArray(res.data)) {
      const run = res.data.find((r) => r.id === runId);
      if (run) {
        last = run;
        const match = Array.isArray(desired)
          ? desired.includes(run.status)
          : run.status === desired;
        if (match) return run;
      }
    }
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(
    `run ${runId} did not reach status ${desired} within ${timeoutMs}ms (last=${JSON.stringify(last)})`,
  );
}

async function waitForNewRunForConfig(slug, configId, sinceCount, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
    if (res.ok && Array.isArray(res.data)) {
      const forCfg = res.data.filter((r) => r.backup_config_id === configId);
      if (forCfg.length > sinceCount) {
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

async function deployViaAPI(name, composeYaml) {
  const b64 = Buffer.from(composeYaml, 'utf-8').toString('base64');
  const res = await apiRequest('POST', '/api/apps/deploy', { name, compose: b64 });
  expect([200, 202], `deploy ${name}: ${JSON.stringify(res.data)}`).toContain(res.status);
}

async function removeApp(slug) {
  try { await apiRequest('DELETE', `/api/apps/${slug}`); } catch {}
}

async function deleteAppConfigsFor(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/backups/configs`);
  if (!res.ok || !Array.isArray(res.data)) return;
  for (const c of res.data) {
    await apiRequest('DELETE', `/api/backups/configs/${c.id}`);
  }
}

async function loginAPI() {
  const r = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  expect(r.ok).toBeTruthy();
}

// Verify a run's file is on disk at <dataDir>/backups/<filename>.
function backupFileOnDisk(run) {
  if (!run || !run.file_path) return null;
  const state = getState();
  const filename = run.file_path.split('/').pop();
  const abs = `${state.dataDir}/backups/${filename}`;
  return existsSync(abs) ? abs : null;
}

// =====================================================================
// MySQL
// =====================================================================
test.describe('Backup strategy: MySQL', () => {
  test.describe.configure({ mode: 'serial' });

  const SLUG = 'e2e-mysql';
  const SERVICE = 'db';
  let container = null;
  let configId = null;

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    await loginAPI();
    await deployViaAPI(SLUG, readFixture('compose-mysql.yml'));
    await waitForAppStatus(SLUG, 'running', 180_000);
    container = findServiceContainer(SLUG, SERVICE);
    expect(container, `${SLUG} ${SERVICE} container`).toBeTruthy();
    // Wait until mysql accepts queries with the configured root password.
    // mysqladmin ping can succeed before the root password is provisioned,
    // so assert we can actually run a query end-to-end.
    await waitForHealthy(
      container,
      `mysql -u root -ptestrootpass123 -N -B -e 'SELECT 1'`,
      120_000,
    );
  });

  test.afterAll(async () => {
    try { await loginAPI(); } catch {}
    await deleteAppConfigsFor(SLUG);
    await removeApp(SLUG);
  });

  test('strategy detection reports mysql service', async () => {
    const res = await apiRequest('GET', `/api/apps/${SLUG}/backups/detect`);
    expect(res.ok).toBeTruthy();
    const strategies = res.data.strategies || [];
    const mysql = strategies.find((s) => s.strategy_type === 'mysql');
    expect(mysql, 'mysql strategy missing').toBeTruthy();
    expect(mysql.available).toBe(true);
    expect(Array.isArray(mysql.services)).toBe(true);
    expect(mysql.services.length).toBeGreaterThan(0);
    // Container name must include the simpledeploy- prefix so docker exec can
    // reach it; this is what the scheduler uses to locate the container.
    expect(mysql.services[0].container_name).toBe(`simpledeploy-${SLUG}-${SERVICE}-1`);
  });

  test('seed -> backup -> drop -> restore -> verify', async () => {
    test.setTimeout(300_000);

    // Seed
    mysqlExec(container, 'testrootpass123', 'testdb',
      "CREATE TABLE IF NOT EXISTS e2e_data(id INT PRIMARY KEY, v VARCHAR(64));");
    mysqlExec(container, 'testrootpass123', 'testdb',
      "DELETE FROM e2e_data; INSERT INTO e2e_data VALUES (1, 'mysql-marker-abc');");
    const seeded = mysqlExec(container, 'testrootpass123', 'testdb',
      "SELECT CONCAT(id,'|',v) FROM e2e_data WHERE id=1;");
    expect(seeded).toContain('1|mysql-marker-abc');

    // Create backup config
    const createRes = await apiRequest('POST', `/api/apps/${SLUG}/backups/configs`, {
      strategy: 'mysql',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    configId = createRes.data.id;

    // Trigger and wait. NOTE: known backend bug — Scheduler does not populate
    // Credentials, so mysqldump will fail auth. This assertion will catch it
    // until the backend is fixed.
    const run = await triggerAndWait(SLUG, configId, 180_000);
    expect(run.status, `mysql backup run status: ${JSON.stringify(run)}`).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();

    // Backup file on disk
    const abs = backupFileOnDisk(run);
    expect(abs, 'backup file must exist on disk').toBeTruthy();

    // Drop and verify gone
    mysqlExec(container, 'testrootpass123', 'testdb', 'DROP TABLE e2e_data;');
    let dropped = false;
    try {
      mysqlExec(container, 'testrootpass123', 'testdb', 'SELECT * FROM e2e_data;');
    } catch {
      dropped = true;
    }
    expect(dropped).toBeTruthy();

    // Restore
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);

    // Poll until data returns
    const deadline = Date.now() + 90_000;
    let got = '';
    while (Date.now() < deadline) {
      try {
        got = mysqlExec(container, 'testrootpass123', 'testdb',
          "SELECT CONCAT(id,'|',v) FROM e2e_data WHERE id=1;");
        if (got.includes('mysql-marker-abc')) break;
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(got).toContain('mysql-marker-abc');
  });
});

// =====================================================================
// MongoDB
// =====================================================================
test.describe('Backup strategy: MongoDB', () => {
  test.describe.configure({ mode: 'serial' });

  const SLUG = 'e2e-mongo';
  const SERVICE = 'db';
  const USER = 'root';
  const PW = 'testrootpass123';
  let container = null;
  let configId = null;

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    await loginAPI();
    await deployViaAPI(SLUG, readFixture('compose-mongo.yml'));
    await waitForAppStatus(SLUG, 'running', 180_000);
    container = findServiceContainer(SLUG, SERVICE);
    expect(container, `${SLUG} ${SERVICE} container`).toBeTruthy();
    await waitForHealthy(
      container,
      `mongosh --quiet --username ${USER} --password ${PW} --authenticationDatabase admin --eval "db.runCommand({ ping: 1 }).ok"`,
      120_000,
    );
  });

  test.afterAll(async () => {
    try { await loginAPI(); } catch {}
    await deleteAppConfigsFor(SLUG);
    await removeApp(SLUG);
  });

  test('strategy detection reports mongo service', async () => {
    const res = await apiRequest('GET', `/api/apps/${SLUG}/backups/detect`);
    expect(res.ok).toBeTruthy();
    const strategies = res.data.strategies || [];
    const mongo = strategies.find((s) => s.strategy_type === 'mongo');
    expect(mongo, 'mongo strategy missing').toBeTruthy();
    expect(mongo.available).toBe(true);
    expect(mongo.services[0].container_name).toBe(`simpledeploy-${SLUG}-${SERVICE}-1`);
  });

  test('seed -> backup -> drop -> restore -> verify', async () => {
    test.setTimeout(300_000);

    // Seed via mongosh
    mongoEval(container, USER, PW,
      'db.getSiblingDB("testdb").e2e.deleteMany({}); db.getSiblingDB("testdb").e2e.insertOne({_id:1, v:"mongo-marker-abc"});');
    const seeded = mongoEval(container, USER, PW,
      'JSON.stringify(db.getSiblingDB("testdb").e2e.findOne({_id:1}))');
    expect(seeded).toContain('mongo-marker-abc');

    // Create config
    const createRes = await apiRequest('POST', `/api/apps/${SLUG}/backups/configs`, {
      strategy: 'mongo',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    configId = createRes.data.id;

    // Trigger. Same Credentials-bug caveat as mysql.
    const run = await triggerAndWait(SLUG, configId, 180_000);
    expect(run.status, `mongo backup run status: ${JSON.stringify(run)}`).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();

    const abs = backupFileOnDisk(run);
    expect(abs, 'mongo backup file must exist on disk').toBeTruthy();

    // Drop collection
    mongoEval(container, USER, PW, 'db.getSiblingDB("testdb").e2e.drop();');
    const afterDrop = mongoEval(container, USER, PW,
      'db.getSiblingDB("testdb").e2e.countDocuments({})');
    expect(afterDrop.trim()).toBe('0');

    // Restore
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);

    // Poll until doc returns
    const deadline = Date.now() + 90_000;
    let found = false;
    while (Date.now() < deadline) {
      try {
        const out = mongoEval(container, USER, PW,
          'JSON.stringify(db.getSiblingDB("testdb").e2e.findOne({_id:1}))');
        if (out.includes('mongo-marker-abc')) { found = true; break; }
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(found).toBeTruthy();
  });
});

// =====================================================================
// Redis
// =====================================================================
test.describe('Backup strategy: Redis', () => {
  test.describe.configure({ mode: 'serial' });

  const SLUG = 'e2e-redis';
  const SERVICE = 'db';
  let container = null;
  let configId = null;

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    await loginAPI();
    await deployViaAPI(SLUG, readFixture('compose-redis.yml'));
    await waitForAppStatus(SLUG, 'running', 180_000);
    container = findServiceContainer(SLUG, SERVICE);
    expect(container, `${SLUG} ${SERVICE} container`).toBeTruthy();
    await waitForHealthy(container, 'redis-cli PING', 60_000);
  });

  test.afterAll(async () => {
    try { await loginAPI(); } catch {}
    await deleteAppConfigsFor(SLUG);
    await removeApp(SLUG);
  });

  test('strategy detection reports redis service', async () => {
    const res = await apiRequest('GET', `/api/apps/${SLUG}/backups/detect`);
    expect(res.ok).toBeTruthy();
    const strategies = res.data.strategies || [];
    const redis = strategies.find((s) => s.strategy_type === 'redis');
    expect(redis, 'redis strategy missing').toBeTruthy();
    expect(redis.available).toBe(true);
    expect(redis.services[0].container_name).toBe(`simpledeploy-${SLUG}-${SERVICE}-1`);
  });

  test('seed -> backup -> delete -> restore -> verify', async () => {
    test.setTimeout(300_000);

    // Seed
    redisCmd(container, 'SET', 'e2e:marker', 'redis-marker-abc');
    expect(redisCmd(container, 'GET', 'e2e:marker')).toBe('redis-marker-abc');

    // Config
    const createRes = await apiRequest('POST', `/api/apps/${SLUG}/backups/configs`, {
      strategy: 'redis',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    configId = createRes.data.id;

    const run = await triggerAndWait(SLUG, configId, 180_000);
    expect(run.status, `redis backup run: ${JSON.stringify(run)}`).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();

    const abs = backupFileOnDisk(run);
    expect(abs, 'redis backup file must exist on disk').toBeTruthy();

    // Delete the key so we can prove restore worked
    redisCmd(container, 'DEL', 'e2e:marker');
    expect(redisCmd(container, 'GET', 'e2e:marker')).toBe('');

    // Restore (redis strategy stops container, cp's in the RDB, starts it)
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);

    // Wait for container to cycle back up after restore
    await waitForContainerState(container, true, 60_000);
    await waitForHealthy(container, 'redis-cli PING', 60_000);

    // Poll until value returns
    const deadline = Date.now() + 60_000;
    let got = '';
    while (Date.now() < deadline) {
      try {
        got = redisCmd(container, 'GET', 'e2e:marker');
        if (got === 'redis-marker-abc') break;
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(got).toBe('redis-marker-abc');
  });
});

// =====================================================================
// SQLite
// =====================================================================
test.describe('Backup strategy: SQLite', () => {
  test.describe.configure({ mode: 'serial' });

  const SLUG = 'e2e-sqlite';
  const SERVICE = 'web';
  const DB = '/data/test.db';
  let container = null;
  let configId = null;

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    await loginAPI();
    await deployViaAPI(SLUG, readFixture('compose-sqlite.yml'));
    await waitForAppStatus(SLUG, 'running', 180_000);
    container = findServiceContainer(SLUG, SERVICE);
    expect(container, `${SLUG} ${SERVICE} container`).toBeTruthy();
    // Wait for the seed command to finish (sqlite install + table create) by
    // probing for the DB file.
    await waitForHealthy(container, `test -f ${DB}`, 120_000);
    // Extra: ensure sqlite3 CLI is present
    await waitForHealthy(container, 'which sqlite3', 60_000);
  });

  test.afterAll(async () => {
    try { await loginAPI(); } catch {}
    await deleteAppConfigsFor(SLUG);
    await removeApp(SLUG);
  });

  test('strategy detection reports sqlite service with path', async () => {
    const res = await apiRequest('GET', `/api/apps/${SLUG}/backups/detect`);
    expect(res.ok).toBeTruthy();
    const strategies = res.data.strategies || [];
    const sqlite = strategies.find((s) => s.strategy_type === 'sqlite');
    expect(sqlite, 'sqlite strategy missing').toBeTruthy();
    expect(sqlite.available).toBe(true);
    expect(sqlite.services[0].container_name).toBe(`simpledeploy-${SLUG}-${SERVICE}-1`);
    // Volume target /data should surface as a candidate path
    expect(Array.isArray(sqlite.services[0].paths)).toBe(true);
    expect(sqlite.services[0].paths).toContain('/data');
  });

  test('seed -> backup -> delete -> restore -> verify', async () => {
    test.setTimeout(300_000);

    // Seed
    sqlite3Eval(container, DB,
      "DELETE FROM e2e_items; INSERT INTO e2e_items(id, v) VALUES (1, 'sqlite-marker-abc');");
    const seeded = sqlite3Eval(container, DB,
      "SELECT id||'|'||v FROM e2e_items WHERE id=1;");
    expect(seeded).toBe('1|sqlite-marker-abc');

    // Create config. The sqlite strategy needs paths — pass the DB file path
    // explicitly via paths (not the volume mount point), since the strategy
    // executes `sqlite3 <path> '.backup ...'` on each path.
    const createRes = await apiRequest('POST', `/api/apps/${SLUG}/backups/configs`, {
      strategy: 'sqlite',
      target: 'local',
      schedule_cron: '0 0 1 1 *',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: JSON.stringify([DB]),
    });
    expect(createRes.ok).toBeTruthy();
    configId = createRes.data.id;

    const run = await triggerAndWait(SLUG, configId, 180_000);
    expect(run.status, `sqlite backup run: ${JSON.stringify(run)}`).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();

    const abs = backupFileOnDisk(run);
    expect(abs, 'sqlite backup file must exist on disk').toBeTruthy();

    // Delete data (keep table, remove rows)
    sqlite3Eval(container, DB, 'DELETE FROM e2e_items;');
    const gone = sqlite3Eval(container, DB, 'SELECT COUNT(*) FROM e2e_items;');
    expect(gone).toBe('0');

    // Restore
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);

    // Poll until row returns
    const deadline = Date.now() + 60_000;
    let got = '';
    while (Date.now() < deadline) {
      try {
        got = sqlite3Eval(container, DB, "SELECT id||'|'||v FROM e2e_items WHERE id=1;");
        if (got === '1|sqlite-marker-abc') break;
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(got).toBe('1|sqlite-marker-abc');
  });
});

