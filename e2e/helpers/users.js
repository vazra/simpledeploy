import { createHash } from 'crypto';
import { expect } from '@playwright/test';
import { apiRequest } from './api.js';

export function b64(s) {
  return Buffer.from(s).toString('base64');
}

// deleteUserByName removes a user left over from an earlier failed run.
export async function deleteUserByName(username) {
  const list = await apiRequest('GET', '/api/users');
  expect(list.status).toBe(200);
  const u = list.data.find((x) => x.username === username);
  if (u) expect((await apiRequest('DELETE', `/api/users/${u.id}`)).status).toBe(200);
}

export async function findUser(username) {
  const list = await apiRequest('GET', '/api/users');
  expect(list.status).toBe(200);
  return list.data.find((u) => u.username === username);
}

// latestDeployEventId returns the newest deploy event id for an app (0 if none).
export async function latestDeployEventId(slug) {
  const res = await apiRequest('GET', `/api/apps/${slug}/events`);
  expect(res.status).toBe(200);
  return res.data.reduce((max, e) => Math.max(max, e.id), 0);
}

// waitForDeployEvent polls until the app records a deploy event newer than
// afterId (deploy, restart, rollback, ...) and returns it. Background work
// started by a 202 or an import ends with such an event.
export async function waitForDeployEvent(slug, afterId, timeoutMs = 90_000) {
  let ev;
  await expect.poll(async () => {
    const res = await apiRequest('GET', `/api/apps/${slug}/events`);
    ev = (res.data || []).filter((e) => e.id > afterId).sort((a, b) => b.id - a.id)[0];
    return ev ? ev.action : null;
  }, { timeout: timeoutMs, intervals: [500] }).not.toBeNull();
  return ev;
}

// waitForAppSettled polls until no deploy is in flight, the app runs, and the
// stored compose hash matches the file on disk. Editing compose labels (IP
// allowlist, endpoints) makes the watcher redeploy about a second later; a
// rollback or deploy started meanwhile is skipped.
export async function waitForAppSettled(slug, timeoutMs = 90_000) {
  await expect.poll(async () => {
    const app = await apiRequest('GET', `/api/apps/${slug}`);
    const compose = await apiRequest('GET', `/api/apps/${slug}/compose`);
    const hash = createHash('sha256').update(String(compose.data)).digest('hex');
    return { deploying: app.data.deploying, status: app.data.Status, hashMatches: app.data.ComposeHash === hash };
  }, { timeout: timeoutMs, intervals: [500] }).toEqual({ deploying: false, status: 'running', hashMatches: true });
}
