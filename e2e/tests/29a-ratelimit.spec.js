// 29a-ratelimit.spec.js
//
// Proxy rate limiting end-to-end on an isolated server. Limits come from the
// app's simpledeploy.ratelimit.* labels. Buckets are keyed by a request
// header so each test (and the readiness poll) starts on a fresh bucket
// without waiting out a window; ip keying is unit-tested in
// internal/proxy/ratelimit_test.go. burst is "0" so the expected sequence
// does not depend on burst semantics.

import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import { fetchViaProxy, waitForProxyStatus } from '../helpers/proxy.js';
import { startIsolatedServer, stopIsolatedServer, deployAt, waitForAppStatusAt } from '../helpers/isolated.js';

test.describe.configure({ mode: 'serial' });

const APP = 'e2e-ratelimit';
const DOMAIN = 'ratelimit-test.local';
const REQUESTS = 5;
const WINDOW_MS = 3_000;
const KEY_HEADER = 'X-RL-Key';

const COMPOSE = [
  'services:',
  '  web:',
  '    image: nginx:alpine',
  '    ports:',
  '      - "8291:80"',
  '    labels:',
  `      simpledeploy.endpoints.0.domain: "${DOMAIN}"`,
  '      simpledeploy.endpoints.0.port: "80"',
  '      simpledeploy.endpoints.0.tls: "off"',
  `      simpledeploy.ratelimit.requests: "${REQUESTS}"`,
  `      simpledeploy.ratelimit.window: "${WINDOW_MS / 1000}s"`,
  '      simpledeploy.ratelimit.burst: "0"',
  `      simpledeploy.ratelimit.by: "header:${KEY_HEADER}"`,
].join('\n');

test.describe('Proxy rate limiting', () => {
  let server = null;
  let api = null;

  const hit = (key) => fetchViaProxy(DOMAIN, '/', { proxyURL: server.proxyURL, headers: { [KEY_HEADER]: key } })
    .then((r) => r.status);

  test.beforeAll(async () => {
    test.setTimeout(240_000);
    let cookie;
    ({ server, cookie, api } = await startIsolatedServer());
    await deployAt(api, APP, COMPOSE);
    await waitForAppStatusAt(server.baseURL, cookie, APP, 'running');
    await waitForProxyStatus(DOMAIN, 200, {
      proxyURL: server.proxyURL, headers: { [KEY_HEADER]: 'readiness' }, bodyContains: 'Welcome to nginx', timeoutMs: 20_000,
    });
  });

  test.afterAll(async () => {
    if (!server) return;
    await api('DELETE', `/api/apps/${APP}`);
    await stopIsolatedServer(server, '29a-ratelimit');
  });

  test('allows exactly the configured requests per window, then 429 with Retry-After', async () => {
    const statuses = [];
    for (let i = 0; i < REQUESTS + 3; i++) statuses.push(await hit('sequence'));
    expect(statuses).toEqual([...Array(REQUESTS).fill(200), 429, 429, 429]);

    const headers = execFileSync('curl', [
      '-sS', '-o', '/dev/null', '-D', '-',
      '--resolve', `${DOMAIN}:${server.proxyPort}:127.0.0.1`,
      '-H', `${KEY_HEADER}: sequence`,
      `http://${DOMAIN}:${server.proxyPort}/`,
    ], { encoding: 'utf-8' });
    expect(headers).toMatch(/^HTTP\/1\.1 429/);
    expect(headers).toMatch(/^Retry-After: 60\r?$/mi);

    // Another key has its own bucket.
    expect(await hit('other-client')).toBe(200);
  });

  test('blocked client recovers once the window elapses, not before', async () => {
    const start = Date.now();
    for (let i = 0; i < REQUESTS; i++) expect(await hit('recovery')).toBe(200);
    expect(await hit('recovery')).toBe(429);

    // 429s do not consume tokens, so polling does not extend the block.
    let recoveredAt = 0;
    while (Date.now() - start < WINDOW_MS + 2_000) {
      if (await hit('recovery') === 200) { recoveredAt = Date.now(); break; }
      await new Promise((r) => setTimeout(r, 250));
    }
    expect(recoveredAt, 'still blocked after the window').toBeGreaterThan(0);
    expect(recoveredAt - start).toBeGreaterThanOrEqual(WINDOW_MS);
  });
});
