import { execFileSync } from 'child_process';
import { join } from 'path';
import { getState } from './auth.js';

export function dbPath() {
  return join(getState().dataDir, 'simpledeploy.db');
}

// The server runs with WAL mode and holds the DB open; external sqlite3
// writes can briefly hit SQLITE_BUSY when a checkpoint is in progress. A
// .timeout prelude makes sqlite3 wait up to 10s for the lock instead of
// erroring immediately.
const BUSY_TIMEOUT_MS = 10_000;

export function sqliteExec(sql) {
  return execFileSync(
    'sqlite3',
    [dbPath(), `.timeout ${BUSY_TIMEOUT_MS}`, sql],
    { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] },
  ).trim();
}

export function sqliteQuery(sql) {
  const out = execFileSync(
    'sqlite3',
    ['-json', dbPath(), `.timeout ${BUSY_TIMEOUT_MS}`, sql],
    { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] },
  ).trim();
  if (!out) return [];
  try { return JSON.parse(out); } catch { return []; }
}

export function tableCount(table) {
  const out = sqliteExec(`SELECT COUNT(*) FROM ${table};`);
  return Number(out);
}

export function getAppId(appSlug) {
  const rows = sqliteQuery(`SELECT id FROM apps WHERE slug='${appSlug}';`);
  if (!rows.length) throw new Error(`app ${appSlug} not found`);
  return rows[0].id;
}

export function insertMetricPoint({ appSlug, cpu, memoryMb, tsSec, tier = 'raw' }) {
  const ts = tsSec || Math.floor(Date.now() / 1000);
  const appId = getAppId(appSlug);
  const mem = Math.round((memoryMb || 0) * 1024 * 1024);
  sqliteExec(
    `INSERT INTO metrics (app_id, container_id, ts, tier, cpu_pct, mem_bytes, mem_limit) VALUES (${appId}, 'e2e-fake', ${ts}, '${tier}', ${cpu}, ${mem}, ${mem * 4});`,
  );
  return appId;
}

export function injectHighCPUWindow(appSlug, durationSec = 120, cpuPct = 95) {
  const now = Math.floor(Date.now() / 1000);
  for (let i = 0; i < 10; i++) {
    insertMetricPoint({
      appSlug,
      cpu: cpuPct,
      memoryMb: 100,
      tsSec: now - (i * 10),
    });
  }
}
