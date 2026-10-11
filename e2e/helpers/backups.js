import { expect } from '@playwright/test';
import { existsSync } from 'fs';
import { apiRequest } from './api.js';
import { getState } from './auth.js';

// Backup config/run helpers shared by the backup specs (12, 13, 27, 28).

const CONFIG_DEFAULTS = {
  target: 'local',
  schedule_cron: '0 0 1 1 *', // yearly; never fires during a run
  target_config_json: '',
  retention_mode: 'count',
  retention_count: 5,
  verify_upload: false,
  pre_hooks: '',
  post_hooks: '',
  paths: '',
};

// createBackupConfig POSTs a config with test defaults and returns its id.
export async function createBackupConfig(slug, fields) {
  const res = await apiRequest('POST', `/api/apps/${slug}/backups/configs`, { ...CONFIG_DEFAULTS, ...fields });
  expect(res.status, `create ${fields.strategy} config for ${slug}: ${JSON.stringify(res.data)}`).toBe(201);
  expect(res.data.id).toBeGreaterThan(0);
  return res.data.id;
}

export async function listRuns(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/backups/runs`);
  expect(res.status, `list runs for ${slug}`).toBe(200);
  return Array.isArray(res.data) ? res.data : [];
}

export async function runsForConfig(slug, configId) {
  return (await listRuns(slug)).filter((r) => r.backup_config_id === configId);
}

export async function pollRunStatus(slug, runId, desired, timeoutMs) {
  const want = Array.isArray(desired) ? desired : [desired];
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    last = (await listRuns(slug)).find((r) => r.id === runId) || last;
    if (last && want.includes(last.status)) return last;
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`run ${runId} did not reach ${want.join('|')} within ${timeoutMs}ms (last=${JSON.stringify(last)})`);
}

// waitForNewRunForConfig returns the first run of configId whose id is not
// in knownIds. Runs are listed per config, so data[0] is not the newest.
export async function waitForNewRunForConfig(slug, configId, knownIds, timeoutMs = 15_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const fresh = (await runsForConfig(slug, configId)).filter((r) => !knownIds.has(r.id));
    if (fresh.length) return fresh.sort((a, b) => b.id - a.id)[0];
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`no new run for config ${configId} in ${slug} within ${timeoutMs}ms`);
}

// triggerAndWait runs configId now and returns the finished run
// (status success or failed).
export async function triggerAndWait(slug, configId, timeoutMs = 180_000) {
  const known = new Set((await runsForConfig(slug, configId)).map((r) => r.id));
  const trig = await apiRequest('POST', `/api/backups/configs/${configId}/run`);
  expect(trig.status, `trigger config ${configId}`).toBe(202);
  const run = await waitForNewRunForConfig(slug, configId, known);
  return pollRunStatus(slug, run.id, ['success', 'failed'], timeoutMs);
}

export async function deleteAppConfigsFor(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/backups/configs`);
  if (!res.ok || !Array.isArray(res.data)) return;
  for (const c of res.data) {
    await apiRequest('DELETE', `/api/backups/configs/${c.id}`);
  }
}

// localBackupPath resolves a local-target run's file. run.file_path is the
// bare filename; the server stores it under <dataDir>/backups.
export function localBackupPath(run) {
  return `${getState().dataDir}/backups/${String(run.file_path).split('/').pop()}`;
}

export function localBackupExists(run) {
  return !!run.file_path && existsSync(localBackupPath(run));
}

// waitForNextSecond resolves once the wall clock is past the second of ts.
// Backup filenames have 1s resolution, so a run triggered after this gets a
// distinct file.
export async function waitForNextSecond(ts) {
  const sec = Math.floor(Date.parse(ts) / 1000);
  while (Math.floor(Date.now() / 1000) <= sec) {
    await new Promise((r) => setTimeout(r, 50));
  }
}

// readOr returns fn() or fallback when it throws (e.g. a table not restored
// yet). For use inside expect.poll.
export function readOr(fn, fallback = '') {
  try {
    return fn();
  } catch {
    return fallback;
  }
}
