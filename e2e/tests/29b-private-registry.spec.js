// 29b-private-registry.spec.js
//
// Verifies that simpledeploy can pull + deploy an app from a private registry
// using credentials stored via the /api/registries endpoint. Spins up a local
// registry:2 container with htpasswd auth, pushes a test image to it, then
// registers credentials and deploys an app referencing the private image.
//
// Requires: docker daemon reachable, and localhost treated as insecure
// registry (default on Docker Desktop — 127.0.0.0/8 is allowed).
//
// Uses the binary already built by global-setup (via getBinaryPath).

import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import { existsSync } from 'fs';
import { startServer, stopServer, getBinaryPath } from '../helpers/server.js';
import { fetchViaProxy } from '../helpers/proxy.js';
import { apiRequestAt } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { startRegistry, pushImage, rmiLocal } from '../helpers/registry.js';
import { listAppContainers } from '../helpers/docker.js';

test.describe.configure({ mode: 'serial' });

function haveDocker() {
  try {
    execFileSync('docker', ['info'], { stdio: ['ignore', 'pipe', 'pipe'] });
    return true;
  } catch { return false; }
}

async function setupAdminAndLogin(baseURL) {
  const setup = await apiRequestAt(baseURL, 'POST', '/api/setup', {
    username: TEST_ADMIN.username,
    password: TEST_ADMIN.password,
    display_name: TEST_ADMIN.displayName,
    email: TEST_ADMIN.email,
  });
  expect(setup.status, 'initial setup should succeed').toBe(201);

  const login = await apiRequestAt(baseURL, 'POST', '/api/auth/login', {
    username: TEST_ADMIN.username,
    password: TEST_ADMIN.password,
  });
  expect(login.status).toBe(200);
  expect(login.setCookie).toBeTruthy();
  return login.setCookie;
}

async function waitForAppStatus(baseURL, cookie, slug, wantStatus, timeoutMs = 180_000) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await apiRequestAt(baseURL, 'GET', `/api/apps/${slug}`, null, cookie);
    last = res;
    const status = res.data && (res.data.Status || res.data.status);
    if (res.ok && status === wantStatus) return res.data;
    await new Promise((r) => setTimeout(r, 1500));
  }
  throw new Error(`app ${slug} did not reach status=${wantStatus}. last: ${JSON.stringify(last && last.data)}`);
}

async function waitForAppStatusAny(baseURL, cookie, slug, wantedStatuses, timeoutMs = 180_000) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await apiRequestAt(baseURL, 'GET', `/api/apps/${slug}`, null, cookie);
    last = res;
    const status = res.data && (res.data.Status || res.data.status);
    if (res.ok && wantedStatuses.includes(status)) return { data: res.data, status };
    await new Promise((r) => setTimeout(r, 1500));
  }
  throw new Error(`app ${slug} did not reach any of ${wantedStatuses.join(',')}. last: ${JSON.stringify(last && last.data)}`);
}

function composeWithImage(image, domain, hostPort, registryName) {
  const lines = [
    'services:',
    '  web:',
    `    image: ${image}`,
    '    ports:',
    `      - "${hostPort}:80"`,
    '    labels:',
    `      simpledeploy.endpoints.0.domain: "${domain}"`,
    '      simpledeploy.endpoints.0.port: "80"',
    '      simpledeploy.endpoints.0.service: "web"',
    '      simpledeploy.endpoints.0.tls: "off"',
  ];
  if (registryName) {
    lines.push(`      simpledeploy.registries: "${registryName}"`);
  }
  return lines.join('\n');
}

test.describe('Private container registry', () => {
  let server = null;
  let cookie = null;
  let registry = null;
  let imageRef = null;
  let registryID = null;

  const appName = 'e2e-priv-registry';
  const domain = 'priv-registry.local';
  const registryName = 'e2e-local-priv';

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    if (!haveDocker()) test.skip(true, 'docker daemon unavailable');
    const bin = getBinaryPath();
    if (!existsSync(bin)) test.skip(true, `binary not found at ${bin}`);

    // 1. Start a local registry with htpasswd auth.
    registry = await startRegistry();

    // 2. Push a test image to it.
    // Use nginx:alpine since the e2e environment typically already has it.
    imageRef = pushImage(registry, 'nginx:alpine', 'e2e/nginx:priv');

    // 3. Start simpledeploy and login.
    server = await startServer(bin);
    cookie = await setupAdminAndLogin(server.baseURL);

    // 4. Register registry credentials via the API.
    const reg = await apiRequestAt(server.baseURL, 'POST', '/api/registries', {
      name: registryName,
      url: registry.host,
      username: registry.user,
      password: registry.pass,
    }, cookie);
    expect(reg.status, `create registry failed: ${JSON.stringify(reg.data)}`).toBe(201);
    registryID = reg.data.id;
    expect(registryID).toBeTruthy();
  });

  test.afterAll(async () => {
    if (server) {
      try { await apiRequestAt(server.baseURL, 'DELETE', `/api/apps/${appName}`, null, cookie); } catch {}
      try {
        if (registryID) {
          await apiRequestAt(server.baseURL, 'DELETE', `/api/registries/${registryID}`, null, cookie);
        }
      } catch {}
      stopServer(server);
    }
    if (registry) registry.stop();
    // Clean up local image tag so subsequent runs re-pull cleanly.
    if (imageRef) rmiLocal(imageRef);
  });

  test('deploy app from private registry succeeds with creds configured', async () => {
    test.setTimeout(300_000);
    // Remove any previously-cached copy locally so the pull must actually go
    // through the private registry.
    rmiLocal(imageRef);

    const yaml = composeWithImage(imageRef, domain, 8391, registryName);
    const deploy = await apiRequestAt(server.baseURL, 'POST', '/api/apps/deploy', {
      name: appName,
      compose: Buffer.from(yaml).toString('base64'),
    }, cookie);
    expect(deploy.status, `deploy failed: ${JSON.stringify(deploy.data)}`).toBe(202);

    await waitForAppStatus(server.baseURL, cookie, appName, 'running');

    const containers = listAppContainers(appName);
    expect(containers.length, 'expected at least one container').toBeGreaterThan(0);

    // Verify the proxy actually serves the app (pull + start succeeded).
    // Give Caddy a moment to register the route.
    await new Promise((r) => setTimeout(r, 2000));
    const res = await fetchViaProxy(domain, '/', { proxyURL: server.proxyURL });
    expect(res.status, `expected 200 via proxy, got ${res.status}`).toBe(200);
    const body = await res.text();
    expect(body).toContain('Welcome to nginx');
  });

  test('deploy fails when registry credentials are removed', async () => {
    test.setTimeout(240_000);
    // Remove app + local image so the next deploy must pull fresh.
    await apiRequestAt(server.baseURL, 'DELETE', `/api/apps/${appName}`, null, cookie);
    await new Promise((r) => setTimeout(r, 2000));

    // Delete the registry credentials.
    const del = await apiRequestAt(
      server.baseURL, 'DELETE', `/api/registries/${registryID}`, null, cookie,
    );
    expect(del.status).toBe(200);
    registryID = null;

    // Wipe the local image copy so docker cannot short-circuit the pull.
    rmiLocal(imageRef);

    // Re-deploy referencing the same private image + (now-missing) registry.
    const yaml = composeWithImage(imageRef, domain, 8391, registryName);
    const deploy = await apiRequestAt(server.baseURL, 'POST', '/api/apps/deploy', {
      name: appName,
      compose: Buffer.from(yaml).toString('base64'),
    }, cookie);
    // Deploy endpoint itself returns 202 (async); failure shows up as app
    // status = "error".
    expect(deploy.status).toBe(202);

    const got = await waitForAppStatusAny(
      server.baseURL, cookie, appName, ['error', 'running'], 120_000,
    );
    expect(
      got.status,
      'deploy should fail with status=error when private image has no creds',
    ).toBe('error');
  });
});
