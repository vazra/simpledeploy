import { test, expect } from '@playwright/test';
import { createHash } from 'crypto';
import { getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, getSessionCookie } from '../helpers/api.js';
import { findServiceContainer, psql } from '../helpers/docker.js';
import { startMinIO, dockerAvailable } from '../helpers/minio.js';
import {
  createBackupConfig,
  deleteAppConfigsFor,
  readOr,
  runsForConfig,
  triggerAndWait,
  waitForNextSecond,
} from '../helpers/backups.js';

// Verifies simpledeploy's S3 backup target end-to-end against a MinIO fixture:
// encrypted target config, backup -> drop -> restore, pre-signed download,
// retention pruning of S3 objects, and failed-auth error reporting.

const POSTGRES_SLUG = 'e2e-postgres';
const BUCKET = 'e2e-backups';

// S3Config in internal/backup/s3.go reads snake_case keys (what the UI
// sends) and still accepts the older Go field names (PascalCase) used here.
// The API encrypts this JSON with master_secret before storage, and
// decrypts at use time.
function s3TargetJSON({ endpoint, bucket, accessKey, secretKey, region = 'us-east-1', prefix = '' }) {
  return JSON.stringify({
    Endpoint: endpoint,
    Bucket: bucket,
    Prefix: prefix,
    AccessKey: accessKey,
    SecretKey: secretKey,
    Region: region,
  });
}

const basename = (key) => String(key).split('/').pop();

test.describe('Backups - S3 target (MinIO fixture)', () => {
  test.describe.configure({ mode: 'serial' });

  test.skip(() => !dockerAvailable(), 'docker not available');

  let minio = null;
  let pgContainer = null;
  let roundtripRun = null;

  const s3Config = (prefix) =>
    s3TargetJSON({
      endpoint: minio.endpoint,
      bucket: BUCKET,
      accessKey: minio.accessKey,
      secretKey: minio.secretKey,
      prefix,
    });

  test.beforeAll(async () => {
    test.setTimeout(180_000);
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.status).toBe(200);

    pgContainer = findServiceContainer(POSTGRES_SLUG, 'db');
    expect(pgContainer, 'e2e-postgres db container must be running').toBeTruthy();

    // simpledeploy runs on the host and reaches MinIO at localhost:<port>.
    minio = await startMinIO({ bucket: BUCKET });
    await deleteAppConfigsFor(POSTGRES_SLUG);
  });

  test.afterAll(async () => {
    try {
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
      await deleteAppConfigsFor(POSTGRES_SLUG);
    } finally {
      if (minio) minio.stop();
    }
  });

  test('1. S3 postgres backup roundtrip: seed, backup, drop, restore, verify', async () => {
    const SELECT_SQL = "SELECT id||'|'||v FROM e2e_s3 WHERE id=1;";
    psql(pgContainer, 'postgres', 'testdb',
      "CREATE TABLE IF NOT EXISTS e2e_s3(id INT, v TEXT); DELETE FROM e2e_s3; INSERT INTO e2e_s3 VALUES (1, 's3-roundtrip-xyz');");
    expect(psql(pgContainer, 'postgres', 'testdb', SELECT_SQL)).toBe('1|s3-roundtrip-xyz');

    const targetJSON = s3Config('roundtrip');
    const cfgId = await createBackupConfig(POSTGRES_SLUG, { strategy: 'postgres', target: 's3', target_config_json: targetJSON });

    // The target config (with the secret key) is stored encrypted.
    const getRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/configs`);
    const stored = getRes.data.find((c) => c.id === cfgId);
    expect(stored.target_config_json).toBeTruthy();
    expect(stored.target_config_json).not.toBe(targetJSON);
    expect(stored.target_config_json).not.toContain(minio.secretKey);

    const run = await triggerAndWait(POSTGRES_SLUG, cfgId);
    expect(run.status, `run error: ${run.error_msg}`).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();
    expect(run.file_path).toMatch(/^roundtrip\/.+\.sql\.gz$/);
    expect(minio.listObjects('roundtrip').map((o) => o.key)).toEqual([basename(run.file_path)]);
    roundtripRun = run;

    psql(pgContainer, 'postgres', 'testdb', 'DROP TABLE e2e_s3;');
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);
    await expect
      .poll(() => readOr(() => psql(pgContainer, 'postgres', 'testdb', SELECT_SQL)), { timeout: 120_000 })
      .toBe('1|s3-roundtrip-xyz');
  });

  test('2. download redirects to a pre-signed URL serving the exact backup', async () => {
    expect(roundtripRun, 'run from test 1').toBeTruthy();

    // Without following redirects, so the 307 is visible.
    const res = await fetch(`${getState().baseURL}/api/backups/runs/${roundtripRun.id}/download`, {
      redirect: 'manual',
      headers: { Cookie: getSessionCookie() },
    });
    expect(res.status).toBe(307);
    const location = res.headers.get('location');
    expect(location).toMatch(/^https?:\/\//);
    expect(location).toMatch(/X-Amz-Signature/);

    // The pre-signed URL needs no session.
    const dl = await fetch(location);
    expect(dl.status).toBe(200);
    const buf = Buffer.from(await dl.arrayBuffer());
    expect(createHash('sha256').update(buf).digest('hex')).toBe(roundtripRun.checksum.replace(/^sha256:/i, ''));
  });

  test('3. bogus S3 credentials produce a failed run with informative error', async () => {
    await deleteAppConfigsFor(POSTGRES_SLUG);
    const cfgId = await createBackupConfig(POSTGRES_SLUG, {
      strategy: 'postgres',
      target: 's3',
      target_config_json: s3TargetJSON({
        endpoint: minio.endpoint,
        bucket: 'definitely-does-not-exist-e2e',
        accessKey: 'bogus-access-key',
        secretKey: 'bogus-secret-key-12345',
        prefix: 'bogus',
      }),
    });

    const run = await triggerAndWait(POSTGRES_SLUG, cfgId, 120_000);
    expect(run.status).toBe('failed');
    expect(run.error_msg).toMatch(/InvalidAccessKeyId|SignatureDoesNotMatch|AccessDenied|NoSuchBucket/i);
  });

  test('4. retention_count=2 prunes the oldest S3 object and its run', async () => {
    await deleteAppConfigsFor(POSTGRES_SLUG);
    const prefix = 'retention';
    const cfgId = await createBackupConfig(POSTGRES_SLUG, {
      strategy: 'postgres',
      target: 's3',
      target_config_json: s3Config(prefix),
      retention_count: 2,
    });

    const runs = [];
    for (let i = 0; i < 3; i++) {
      if (runs.length) await waitForNextSecond(runs[runs.length - 1].finished_at);
      const run = await triggerAndWait(POSTGRES_SLUG, cfgId);
      expect(run.status, `run error: ${run.error_msg}`).toBe('success');
      runs.push(run);
    }
    expect(new Set(runs.map((r) => r.file_path)).size, 'distinct object keys').toBe(3);

    // Pruning runs right after the third run is marked success.
    const kept = runs.slice(1);
    await expect
      .poll(() => minio.listObjects(prefix).map((o) => o.key).sort())
      .toEqual(kept.map((r) => basename(r.file_path)).sort());

    // A pruned run must not stay listed as a downloadable/restorable backup.
    await expect
      .poll(async () => (await runsForConfig(POSTGRES_SLUG, cfgId)).map((r) => r.id).sort())
      .toEqual(kept.map((r) => r.id).sort());
  });
});
