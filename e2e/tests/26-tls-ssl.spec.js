// 26-tls-ssl.spec.js
//
// TLS end-to-end on an isolated server with tls.mode=local (Caddy internal
// CA). One app, two endpoints: tls-local.test keeps the default tls (cert
// from the internal CA); custom-tls.test is switched to an uploaded cert.
// The plain-HTTP (tls=off) path is covered by 03-deploy and 08-app-endpoints.

import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import { mkdtempSync, readFileSync, rmSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import { curlHTTPS, openSSLGetCert } from '../helpers/proxy.js';
import { startIsolatedServer, stopIsolatedServer, deployAt, waitForAppStatusAt } from '../helpers/isolated.js';

test.describe.configure({ mode: 'serial' });

const APP = 'e2e-tls';
const LOCAL_DOMAIN = 'tls-local.test';
const CUSTOM_DOMAIN = 'custom-tls.test';

const COMPOSE = [
  'services:',
  '  web:',
  '    image: nginx:alpine',
  '    ports:',
  '      - "8191:80"',
  '    labels:',
  `      simpledeploy.endpoints.0.domain: "${LOCAL_DOMAIN}"`,
  '      simpledeploy.endpoints.0.port: "80"',
  `      simpledeploy.endpoints.1.domain: "${CUSTOM_DOMAIN}"`,
  '      simpledeploy.endpoints.1.port: "80"',
].join('\n');

async function pollUntil(fn, ok, timeoutMs, what) {
  const deadline = Date.now() + timeoutMs;
  let last;
  while (Date.now() < deadline) {
    last = fn();
    if (ok(last)) return last;
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`${what} not reached within ${timeoutMs}ms; last: ${JSON.stringify(last).slice(0, 300)}`);
}

test.describe('TLS mode=local', () => {
  let server = null;
  let api = null;

  test.beforeAll(async () => {
    test.setTimeout(240_000);
    let cookie;
    ({ server, cookie, api } = await startIsolatedServer({ tlsMode: 'local' }));
    await deployAt(api, APP, COMPOSE);
    await waitForAppStatusAt(server.baseURL, cookie, APP, 'running');
  });

  test.afterAll(async () => {
    if (!server) return;
    await api('DELETE', `/api/apps/${APP}`);
    await stopIsolatedServer(server, '26-tls');
  });

  test('default endpoint serves the app over HTTPS with a Caddy internal CA cert', async () => {
    await pollUntil(
      () => curlHTTPS(LOCAL_DOMAIN, server.proxyPort, '/'),
      (r) => r.status === 200 && r.body.includes('Welcome to nginx'),
      30_000, `HTTPS 200 from ${LOCAL_DOMAIN}`,
    );
    const cert = openSSLGetCert(LOCAL_DOMAIN, server.proxyPort);
    expect(cert.error, `openssl: ${cert.error}`).toBeUndefined();
    expect(cert.issuer).toMatch(/Caddy Local Authority/);
  });

  test('uploaded cert is served for its domain only', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'sd-e2e-tls-custom-'));
    try {
      const crt = join(dir, 'custom.crt');
      const key = join(dir, 'custom.key');
      execFileSync('openssl', [
        'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '30',
        '-keyout', key, '-out', crt,
        '-subj', `/CN=${CUSTOM_DOMAIN}`,
        '-addext', `subjectAltName=DNS:${CUSTOM_DOMAIN}`,
      ], { stdio: ['ignore', 'pipe', 'pipe'] });

      const up = await api('PUT', `/api/apps/${APP}/certs/${CUSTOM_DOMAIN}`, {
        cert: readFileSync(crt, 'utf-8'),
        key: readFileSync(key, 'utf-8'),
      });
      expect(up.status, JSON.stringify(up.data)).toBe(200);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }

    const app = await api('GET', `/api/apps/${APP}`);
    expect(app.status).toBe(200);
    const endpoints = app.data.endpoints.map((e) => (e.domain === CUSTOM_DOMAIN ? { ...e, tls: 'custom' } : e));
    expect(endpoints.map((e) => e.domain).sort()).toEqual([CUSTOM_DOMAIN, LOCAL_DOMAIN]);
    const put = await api('PUT', `/api/apps/${APP}/endpoints`, endpoints);
    expect(put.status, JSON.stringify(put.data)).toBe(200);

    // Caddy's internal leaf certs have an empty subject, so a CN match means
    // the uploaded PEM is the one being served.
    const cert = await pollUntil(
      () => openSSLGetCert(CUSTOM_DOMAIN, server.proxyPort),
      (c) => (c.subject || '').includes(CUSTOM_DOMAIN),
      30_000, `uploaded cert served for ${CUSTOM_DOMAIN}`,
    );
    expect(cert.issuer).not.toMatch(/Caddy/);

    const res = curlHTTPS(CUSTOM_DOMAIN, server.proxyPort, '/');
    expect(res.status, res.error).toBe(200);
    expect(res.body).toContain('Welcome to nginx');

    // The other endpoint keeps its internal CA cert.
    expect(openSSLGetCert(LOCAL_DOMAIN, server.proxyPort).issuer).toMatch(/Caddy Local Authority/);
  });
});
