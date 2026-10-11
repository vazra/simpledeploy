// 29b-private-registry.spec.js
//
// Deploys from a private registry using credentials stored via
// /api/registries. A local registry:2 with htpasswd auth serves a pushed
// test image; the app references it through simpledeploy.registries.
// Requires localhost to be an insecure registry (Docker default for
// 127.0.0.0/8).

import { test, expect } from '@playwright/test';
import { waitForProxyStatus } from '../helpers/proxy.js';
import { startRegistry, pushImage, rmiLocal } from '../helpers/registry.js';
import { findServiceContainer, containerImage } from '../helpers/docker.js';
import { startIsolatedServer, stopIsolatedServer, deployAt, waitForAppStatusAt } from '../helpers/isolated.js';

test.describe.configure({ mode: 'serial' });

const APP = 'e2e-priv-registry';
const DOMAIN = 'priv-registry.local';
const REGISTRY_NAME = 'e2e-local-priv';

function composeWithImage(image) {
  return [
    'services:',
    '  web:',
    `    image: ${image}`,
    '    ports:',
    '      - "8391:80"',
    '    labels:',
    `      simpledeploy.endpoints.0.domain: "${DOMAIN}"`,
    '      simpledeploy.endpoints.0.port: "80"',
    '      simpledeploy.endpoints.0.tls: "off"',
    `      simpledeploy.registries: "${REGISTRY_NAME}"`,
  ].join('\n');
}

test.describe('Private container registry', () => {
  let server = null;
  let cookie = null;
  let api = null;
  let registry = null;
  let imageRef = null;
  let registryID = null;

  test.beforeAll(async () => {
    test.setTimeout(300_000);
    registry = await startRegistry();
    imageRef = pushImage(registry, 'nginx:alpine', 'e2e/nginx:priv');

    ({ server, cookie, api } = await startIsolatedServer());
    const reg = await api('POST', '/api/registries', {
      name: REGISTRY_NAME,
      url: registry.host,
      username: registry.user,
      password: registry.pass,
    });
    expect(reg.status, JSON.stringify(reg.data)).toBe(201);
    registryID = reg.data.id;
  });

  test.afterAll(async () => {
    if (server) {
      await api('DELETE', `/api/apps/${APP}`);
      await stopIsolatedServer(server, '29b-private-registry');
    }
    if (registry) registry.stop();
    if (imageRef) rmiLocal(imageRef);
  });

  test('deploy pulls the private image with the stored credentials', async () => {
    test.setTimeout(300_000);
    // No local copy, so the image must come from the registry.
    rmiLocal(imageRef);

    await deployAt(api, APP, composeWithImage(imageRef));
    await waitForAppStatusAt(server.baseURL, cookie, APP, 'running');

    expect(containerImage(findServiceContainer(APP, 'web'))).toBe(imageRef);
    await waitForProxyStatus(DOMAIN, 200, { proxyURL: server.proxyURL, bodyContains: 'Welcome to nginx', timeoutMs: 20_000 });
  });

  test('deploy fails on registry auth once the credentials are removed', async () => {
    test.setTimeout(240_000);
    const del = await api('DELETE', `/api/apps/${APP}`);
    expect(del.status).toBe(200);
    const delReg = await api('DELETE', `/api/registries/${registryID}`);
    expect(delReg.status).toBe(200);
    rmiLocal(imageRef);

    await deployAt(api, APP, composeWithImage(imageRef));
    const app = await waitForAppStatusAt(server.baseURL, cookie, APP, ['error', 'running'], 120_000);
    expect(app.Status, 'deploy without credentials should fail').toBe('error');

    // The failure must be the registry refusing the anonymous pull, not some
    // unrelated error (port clash, compose error, timeout).
    const events = await api('GET', `/api/apps/${APP}/events`);
    expect(events.status).toBe(200);
    const failed = events.data.find((e) => e.action === 'deploy_failed');
    expect(failed, JSON.stringify(events.data)).toBeTruthy();
    expect(failed.detail).toMatch(/no basic auth credentials|unauthorized|authentication required/i);
  });
});
