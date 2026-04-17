import { test, expect } from '@playwright/test';
import { createGunzip } from 'zlib';
import { Readable } from 'stream';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, apiDownload, getSessionCookie } from '../helpers/api.js';
import { findServiceContainer, psql } from '../helpers/docker.js';
import { startMinIO, dockerAvailable } from '../helpers/minio.js';

// Verifies simpledeploy's S3 backup target end-to-end against a MinIO fixture.
// Covers: create config with encrypted S3 target, backup roundtrip (backup →
// drop → restore), gzipped SQL payload validation, pre-signed download URL,
// retention pruning of S3 objects, and failed-auth error reporting.

const POSTGRES_SLUG = 'e2e-postgres';
const BUCKET = 'e2e-backups';

// S3Config in internal/backup/s3.go has NO json tags, so encoding/json
// serializes fields using Go field names (PascalCase). The API encrypts
// this JSON with master_secret before storage, and decrypts at use time.
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

async function pollRunStatus(slug, runId, desiredStatuses, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
    if (res.ok && Array.isArray(res.data)) {
      const run = res.data.find((r) => r.id === runId);
      if (run) {
        last = run;
        if (desiredStatuses.includes(run.status)) return run;
      }
    }
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`run ${runId} did not reach ${desiredStatuses.join('|')} in ${timeoutMs}ms (last=${JSON.stringify(last)})`);
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

async function triggerAndWait(slug, configId, { timeoutMs = 180_000, allowFail = true } = {}) {
  const before = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
  const beforeCount = Array.isArray(before.data)
    ? before.data.filter((r) => r.backup_config_id === configId).length
    : 0;
  const trig = await apiRequest('POST', `/api/backups/configs/${configId}/run`);
  expect([200, 202]).toContain(trig.status);
  const newest = await waitForNewRunForConfig(slug, configId, beforeCount, 15_000);
  const desired = allowFail ? ['success', 'failed'] : ['success'];
  return await pollRunStatus(slug, newest.id, desired, timeoutMs);
}

async function deleteAppConfigsFor(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/backups/configs`);
  if (!res.ok || !Array.isArray(res.data)) return;
  for (const c of res.data) {
    await apiRequest('DELETE', `/api/backups/configs/${c.id}`);
  }
}

async function streamToBuffer(stream) {
  const chunks = [];
  for await (const chunk of stream) chunks.push(chunk);
  return Buffer.concat(chunks);
}

async function gunzipBuffer(buf) {
  const gz = createGunzip();
  const src = Readable.from(buf);
  src.pipe(gz);
  return await streamToBuffer(gz);
}

test.describe('Backups - S3 target (MinIO fixture)', () => {
  test.describe.configure({ mode: 'serial' });

  // Skip entire suite if docker is unavailable.
  test.skip(() => !dockerAvailable(), 'docker not available');

  let minio = null;
  let pgContainer = null;

  test.beforeAll(async () => {
    test.setTimeout(180_000);
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.ok).toBeTruthy();

    pgContainer = findServiceContainer(POSTGRES_SLUG, 'db');
    expect(pgContainer, 'e2e-postgres db container must be running').toBeTruthy();

    // Start MinIO and create bucket. simpledeploy is running on the host so
    // it reaches MinIO at http://localhost:<port>; no container networking
    // gymnastics needed for the server. The `mc` subprocess runs inside its
    // own container and uses host.docker.internal.
    minio = await startMinIO({ bucket: BUCKET });

    // Clean any configs from the earlier backups spec.
    await deleteAppConfigsFor(POSTGRES_SLUG);
  });

  test.afterAll(async () => {
    try {
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
      await deleteAppConfigsFor(POSTGRES_SLUG);
    } catch {}
    if (minio) {
      try { minio.stop(); } catch {}
    }
  });

  test('1. S3 postgres backup roundtrip: seed, backup, drop, restore, verify', async () => {
    test.setTimeout(300_000);

    // Seed data
    psql(pgContainer, 'postgres', 'testdb',
      "CREATE TABLE IF NOT EXISTS e2e_s3(id INT, v TEXT); DELETE FROM e2e_s3; INSERT INTO e2e_s3 VALUES (1, 's3-roundtrip-xyz');");
    expect(psql(pgContainer, 'postgres', 'testdb', "SELECT id||'|'||v FROM e2e_s3 WHERE id=1;"))
      .toContain('1|s3-roundtrip-xyz');

    const targetJSON = s3TargetJSON({
      endpoint: minio.endpoint,
      bucket: BUCKET,
      accessKey: minio.accessKey,
      secretKey: minio.secretKey,
      prefix: 'roundtrip',
    });

    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'postgres',
      target: 's3',
      schedule_cron: '0 0 1 1 *',
      target_config_json: targetJSON,
      retention_mode: 'count',
      retention_count: 5,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok, `create config: ${JSON.stringify(createRes.data)}`).toBeTruthy();
    const cfgId = createRes.data.id;
    expect(cfgId).toBeGreaterThan(0);

    // Server stores the target_config_json encrypted; GET should return
    // the encrypted blob (not the plaintext we sent).
    const getRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/configs`);
    const stored = Array.isArray(getRes.data) ? getRes.data.find((c) => c.id === cfgId) : null;
    expect(stored).toBeTruthy();
    expect(stored.target_config_json).toBeTruthy();
    expect(stored.target_config_json).not.toBe(targetJSON);

    // Trigger backup and wait for success
    const run = await triggerAndWait(POSTGRES_SLUG, cfgId, { timeoutMs: 180_000, allowFail: false });
    expect(run.status).toBe('success');
    expect(run.size_bytes).toBeGreaterThan(0);
    expect(run.checksum).toBeTruthy();
    expect(run.file_path).toBeTruthy();
    // For S3 target, file_path is the object key (may include prefix).
    expect(run.file_path).toMatch(/roundtrip\//);

    // Drop data, restore, verify
    psql(pgContainer, 'postgres', 'testdb', 'DROP TABLE e2e_s3;');
    const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
    expect(restoreRes.status).toBe(202);

    const deadline = Date.now() + 120_000;
    let restored = '';
    while (Date.now() < deadline) {
      try {
        restored = psql(pgContainer, 'postgres', 'testdb', "SELECT id||'|'||v FROM e2e_s3 WHERE id=1;");
        if (restored.includes('s3-roundtrip-xyz')) break;
      } catch {}
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(restored).toContain('1|s3-roundtrip-xyz');

    // Save for next test
    test.info().annotations.push({ type: 'configId', description: String(cfgId) });
  });

  test('2. S3 upload is a valid gzipped SQL dump containing expected markers', async () => {
    test.setTimeout(60_000);

    // List objects in the bucket under the roundtrip prefix
    const objs = minio.listObjects('roundtrip');
    expect(objs.length, `expected at least one object, got ${JSON.stringify(objs)}`).toBeGreaterThanOrEqual(1);

    // Pick an object key ending in .sql.gz (postgres strategy)
    const sqlGz = objs.find((o) => String(o.key).endsWith('.sql.gz')) || objs[0];
    expect(sqlGz).toBeTruthy();

    // Download via mc cat to stdout, capture as buffer.
    // Use the mc subprocess through our helper; key is relative to bucket.
    // mc path: local/<bucket>/<key-with-prefix>
    // listObjects returned keys relative to the prefix scope; build a full path.
    // Safer: just download through the simpledeploy pre-signed URL path in test 3.
    // Here, use `mc cat` with full path reconstructed from listObjects call above.
    const fullKey = String(sqlGz.key).startsWith('roundtrip/')
      ? sqlGz.key
      : `roundtrip/${String(sqlGz.key).replace(/^\//, '')}`;
    const catOut = minio.mc(['cat', `local/${BUCKET}/${fullKey}`], { encoding: 'buffer' });
    const gzBuf = Buffer.isBuffer(catOut) ? catOut : Buffer.from(catOut, 'binary');
    expect(gzBuf.length).toBeGreaterThan(0);

    // gzip magic bytes 0x1f 0x8b
    expect(gzBuf[0]).toBe(0x1f);
    expect(gzBuf[1]).toBe(0x8b);

    const plain = await gunzipBuffer(gzBuf);
    const text = plain.toString('utf-8');
    // pg_dump output contains schema markers and our table
    expect(text).toMatch(/(CREATE TABLE|COPY )/);
    expect(text).toContain('e2e_s3');
  });

  test('3. pre-signed download URL: 307 redirect returns valid gzip', async () => {
    test.setTimeout(60_000);

    // Find a successful s3 run
    const runsRes = await apiRequest('GET', `/api/apps/${POSTGRES_SLUG}/backups/runs`);
    expect(runsRes.ok).toBeTruthy();
    const run = runsRes.data.find((r) => r.status === 'success' && r.file_path && r.file_path.endsWith('.sql.gz'));
    expect(run, 'need a successful S3 run to test download').toBeTruthy();

    // Issue download WITHOUT following redirects so we can assert 307.
    const state = getState();
    const cookie = getSessionCookie();
    const res = await fetch(`${state.baseURL}/api/backups/runs/${run.id}/download`, {
      method: 'GET',
      redirect: 'manual',
      headers: cookie ? { Cookie: cookie } : {},
    });
    expect(res.status).toBe(307);
    const location = res.headers.get('location');
    expect(location).toBeTruthy();
    expect(location).toMatch(/^https?:\/\//);
    expect(location).toMatch(/X-Amz-Signature|X-Amz-Credential/);

    // Follow the pre-signed URL directly (no auth cookie needed — it's signed).
    const dl = await fetch(location);
    expect(dl.ok, `presigned fetch failed: ${dl.status}`).toBeTruthy();
    const buf = Buffer.from(await dl.arrayBuffer());
    expect(buf.length).toBeGreaterThan(0);
    expect(buf[0]).toBe(0x1f);
    expect(buf[1]).toBe(0x8b);
    // Gunzip should succeed
    const plain = await gunzipBuffer(buf);
    expect(plain.length).toBeGreaterThan(0);
  });

  test('4. retention_count=2 prunes old S3 objects', async () => {
    test.setTimeout(600_000);

    // Fresh bucket prefix so we can count in isolation
    await deleteAppConfigsFor(POSTGRES_SLUG);

    const prefix = 'retention';
    const targetJSON = s3TargetJSON({
      endpoint: minio.endpoint,
      bucket: BUCKET,
      accessKey: minio.accessKey,
      secretKey: minio.secretKey,
      prefix,
    });

    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'postgres',
      target: 's3',
      schedule_cron: '0 0 1 1 *',
      target_config_json: targetJSON,
      retention_mode: 'count',
      retention_count: 2,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    const cfgId = createRes.data.id;

    // Trigger 3 backups sequentially (each pg strategy uses a timestamped filename)
    for (let i = 0; i < 3; i++) {
      const run = await triggerAndWait(POSTGRES_SLUG, cfgId, { timeoutMs: 180_000, allowFail: false });
      expect(run.status).toBe('success');
      // Ensure distinct timestamps in the filename (YYYYMMDD-HHMMSS granularity).
      await new Promise((r) => setTimeout(r, 1500));
    }

    // List objects under this prefix. Retention prunes to retention_count=2.
    // Poll briefly in case pruning runs slightly after the run marks success.
    let objs = [];
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      objs = minio.listObjects(prefix);
      if (objs.length <= 2) break;
      await new Promise((r) => setTimeout(r, 1_000));
    }
    expect(objs.length, `expected <=2 objects, got ${objs.length}: ${JSON.stringify(objs)}`).toBeLessThanOrEqual(2);
    expect(objs.length).toBeGreaterThanOrEqual(1);
  });

  test('5. bogus S3 credentials produce a failed run with informative error', async () => {
    test.setTimeout(180_000);

    await deleteAppConfigsFor(POSTGRES_SLUG);

    const targetJSON = s3TargetJSON({
      endpoint: minio.endpoint,
      bucket: 'definitely-does-not-exist-e2e',
      accessKey: 'bogus-access-key',
      secretKey: 'bogus-secret-key-12345',
      prefix: 'bogus',
    });

    const createRes = await apiRequest('POST', `/api/apps/${POSTGRES_SLUG}/backups/configs`, {
      strategy: 'postgres',
      target: 's3',
      schedule_cron: '0 0 1 1 *',
      target_config_json: targetJSON,
      retention_mode: 'count',
      retention_count: 5,
      verify_upload: false,
      pre_hooks: '',
      post_hooks: '',
      paths: '',
    });
    expect(createRes.ok).toBeTruthy();
    const cfgId = createRes.data.id;

    const run = await triggerAndWait(POSTGRES_SLUG, cfgId, { timeoutMs: 120_000, allowFail: true });
    expect(run.status).toBe('failed');
    expect(run.error_msg || '').toBeTruthy();
    // Should mention s3/bucket/auth/access/credential somewhere
    expect(run.error_msg.toLowerCase()).toMatch(/(s3|bucket|access|auth|credential|signature|key|denied)/);
  });

  test('6. UI shows S3 option in backup wizard', async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/apps/${POSTGRES_SLUG}`);
    await page.getByRole('button', { name: /backups/i }).click();

    const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
    await configBtn.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Advance to step 2 (storage target). Strategy step first.
    const pgBtn = dialog.getByText(/postgresql/i).first();
    if (await pgBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await pgBtn.click();
    }
    await dialog.getByRole('button', { name: /next/i }).click();

    // S3 option should be visible as a storage target.
    await expect(dialog.getByText(/s3|object storage/i).first()).toBeVisible({ timeout: 5_000 });
  });
});
