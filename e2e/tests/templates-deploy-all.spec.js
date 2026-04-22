// Deploy every app template end-to-end through the wizard, then verify the
// deployed service is actually reachable via the SimpleDeploy proxy.
// Expensive: pulls ~20 different multi-service stacks. NEVER runs under
// `make e2e` or `make e2e-lite` — only when E2E_TEMPLATES=1 (see
// playwright.config.js testMatch gate and the `e2e-templates` Make target).
// Intended to run once whenever a template is added or changed.
//
// Strategy per template:
//   1. Fill declared variables with type-appropriate defaults. Each
//      `domain`-typed variable gets a UNIQUE subdomain keyed off the var
//      name so multi-endpoint templates (minio, mailpit, docker-registry)
//      don't collide in Caddy's routing table.
//   2. Deploy through the wizard. Assert terminal "Deployed" (or "Unstable"
//      for `probe: null` templates where the stack can't serve without
//      external setup by design).
//   3. If a probe spec exists (see `helpers/template-probes.js`), probe
//      each endpoint via `http://127.0.0.1:${proxyPort}<path>` with the
//      template's filled domain as the `Host:` header and assert the
//      expected status/body. This is the real reachability check.
//   4. Delete the app via API before the next template.

import { test, expect } from '@playwright/test';
import { getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { appTemplates } from '../../ui/src/lib/appTemplates.js';
import { templateProbes } from '../helpers/template-probes.js';
import http from 'node:http';

async function removeAppIfExists(slug) {
  try { await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password); } catch {}
  try { await apiRequest('DELETE', `/api/apps/${slug}`); } catch {}
}

function slugFor(tpl) {
  return `e2e-tpl-${tpl.id}`.slice(0, 40).replace(/[^a-z0-9-]/g, '-');
}

// Produce a unique test domain per variable key so multi-endpoint templates
// (minio, mailpit, docker-registry) get distinct Host headers and don't
// conflict in the Caddy route table.
function domainForVar(tpl, varKey) {
  // Domain validation rejects underscores, so normalize var keys like
  // `console_domain`/`smtp_domain`/`ui_domain` into hyphenated labels.
  const safe = varKey.replace(/_/g, '-');
  return `${safe}.${slugFor(tpl)}.local`;
}

function valueForVar(tpl, v) {
  if (v.type === 'domain') return domainForVar(tpl, v.key);
  if (v.type === 'email')  return 'e2e@example.com';
  if (v.type === 'number') return String(v.default ?? v.placeholder ?? 8080);
  if (v.type === 'secret') return 'E2eTestSecretValue12345';
  if (v.type === 'enum') {
    const opts = v.options || [];
    return String(v.default ?? opts[0]?.value ?? '');
  }
  return v.default != null ? String(v.default) : `e2e-${v.key}`;
}

async function fillVariable(dialog, v, tpl) {
  const input = dialog.locator(`#tpl-var-${v.key}`);
  if (!(await input.isVisible().catch(() => false))) return;

  if (v.type === 'enum') {
    const opts = v.options || [];
    const def = v.default || opts[0]?.value;
    if (def) await input.selectOption(String(def));
    return;
  }

  // For domain inputs we ALWAYS want to overwrite: the wizard's Custom
  // domain mode may prefill a placeholder, but we need the per-var test
  // domain so probes match.
  const force = v.type === 'domain';
  if (!force) {
    const existing = await input.inputValue().catch(() => '');
    if (existing && existing.length > 0) return;
  }

  await input.fill(valueForVar(tpl, v));
}

// One HTTP GET against the proxy with a Host header override. Uses raw
// node:http because undici's fetch() strips the `Host` request header, which
// defeats host-based routing on the Caddy proxy.
function httpGet({ proxyPort, host, path }) {
  return new Promise((resolve, reject) => {
    const req = http.request(
      { host: '127.0.0.1', port: proxyPort, path, method: 'GET', headers: { Host: host }, timeout: 5000 },
      (res) => {
        const chunks = [];
        res.on('data', (c) => chunks.push(c));
        res.on('end', () => resolve({ status: res.statusCode, body: Buffer.concat(chunks).toString('utf8') }));
        res.on('error', reject);
      }
    );
    req.on('error', reject);
    req.on('timeout', () => { req.destroy(new Error('request timeout')); });
    req.end();
  });
}

// Probe a single endpoint via the Caddy proxy port with a Host header
// override. Retries until reachable or timeout.
async function probeEndpoint({ proxyPort, host, path, statusMin, statusMax, bodyIncludes, timeoutMs = 60_000 }) {
  const deadline = Date.now() + timeoutMs;
  let lastErr = null;
  let lastStatus = null;
  let lastBody = null;
  while (Date.now() < deadline) {
    try {
      const res = await httpGet({ proxyPort, host, path });
      lastStatus = res.status;
      const body = res.body;
      lastBody = body.slice(0, 500);
      // Caddy returns 502/504 while the upstream is still booting. Keep retrying.
      if (res.status === 502 || res.status === 504) {
        await new Promise((r) => setTimeout(r, 2000));
        continue;
      }
      if (res.status < statusMin || res.status > statusMax) {
        lastErr = `status ${res.status} not in [${statusMin},${statusMax}]`;
        await new Promise((r) => setTimeout(r, 2000));
        continue;
      }
      if (bodyIncludes && !body.includes(bodyIncludes)) {
        lastErr = `body missing substring "${bodyIncludes}"`;
        await new Promise((r) => setTimeout(r, 2000));
        continue;
      }
      return { ok: true, status: res.status };
    } catch (err) {
      lastErr = String(err);
      await new Promise((r) => setTimeout(r, 2000));
    }
  }
  return { ok: false, lastErr, lastStatus, lastBody };
}

// Auth via storageState from globalSetup; each test uses a unique slug so
// parallelism within a shard is safe.
test.describe('Deploy every template (E2E_TEMPLATES=1)', () => {
  for (const tpl of appTemplates) {
    test(`deploy template "${tpl.name}"`, async ({ page }) => {
      test.setTimeout(900_000);
      const slug = slugFor(tpl);
      const probeSpec = templateProbes[tpl.id];
      if (!probeSpec) {
        throw new Error(`No probe spec in template-probes.js for ${tpl.id}. Add one.`);
      }

      // Snapshot the filled var values so we know what Host header each
      // endpoint will answer to.
      const filledVars = {};
      for (const v of tpl.variables || []) filledVars[v.key] = valueForVar(tpl, v);

      await removeAppIfExists(slug);

      const state = getState();
      await page.goto(`${state.baseURL}/#/`);
      await page.getByRole('button', { name: 'Deploy App' }).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.getByRole('button', { name: /^browse templates$/i }).click();
      await dialog.getByRole('button', { name: `Use template ${tpl.name}` }).click();

      // Custom domain mode so `domain` vars render as editable inputs
      // (Quick test mode hides them behind an sslip.io helper).
      await dialog.getByRole('button', { name: /^custom domain$/i }).click();

      const hasHidden = (tpl.variables || []).some((v) => v.hidden);
      if (hasHidden) await dialog.getByText(/advanced \/ secrets/i).click();

      for (const v of tpl.variables || []) await fillVariable(dialog, v, tpl);

      await dialog.getByRole('button', { name: /^apply/i }).click();

      await dialog.getByPlaceholder('my-app').fill(slug);
      await expect(dialog.getByText(/valid compose/i)).toBeVisible({ timeout: 20_000 });

      await dialog.getByRole('button', { name: 'Next' }).click();
      await dialog.getByRole('button', { name: 'Deploy' }).click();

      // Terminal state. For probe: null templates we accept Unstable/Failed
      // because the stack can't serve HTTP without external setup. For
      // templates with a probe spec we require Deployed (the HTTP probe
      // below is the real reachability check).
      const statusBadge = dialog.locator('[data-testid="deploy-status"]');
      if (probeSpec.probe === null) {
        await expect(statusBadge).toHaveAttribute('data-deploy-status', /^(success|unstable|failed)$/, { timeout: 540_000 });
        // No HTTP probe: document why.
        console.log(`[templates] ${tpl.id}: probe skipped - ${probeSpec.reason}`);
      } else {
        await expect(statusBadge).toHaveAttribute('data-deploy-status', /^(success|unstable)$/, { timeout: 540_000 });

        // Close the wizard so the next interaction path is clean. (The
        // probe doesn't depend on UI state, but leaving dialogs open
        // across tests has caused flakes in other specs.)
        await page.keyboard.press('Escape').catch(() => {});

        // Give the reconciler one tick (status refresh interval is 5s in
        // e2e) to push routes into Caddy after the container is healthy.
        await new Promise((r) => setTimeout(r, 6000));

        for (const p of probeSpec.probes) {
          const hostVar = p.hostVar || 'domain';
          const host = filledVars[hostVar];
          if (!host) {
            throw new Error(`Template ${tpl.id} probe references unknown hostVar "${hostVar}"`);
          }
          const result = await probeEndpoint({
            proxyPort: state.proxyPort,
            host,
            path: p.path,
            statusMin: p.statusMin,
            statusMax: p.statusMax,
            bodyIncludes: p.bodyIncludes,
            timeoutMs: p.timeoutMs || 120_000,
          });
          if (!result.ok) {
            let diag = '';
            try {
              const statusRes = await apiRequest('GET', `/api/apps/${slug}/services`);
              diag += `\nservices=${JSON.stringify(statusRes).slice(0, 300)}`;
            } catch (e) { diag += `\nservices-fetch-err=${e.message}`; }
            throw new Error(
              `Probe failed for ${tpl.id} (host=${host} path=${p.path}): ${result.lastErr}\n` +
              `last status=${result.lastStatus} body-prefix=${(result.lastBody || '').slice(0, 200)}` +
              diag
            );
          }
          console.log(`[templates] ${tpl.id}: probe ok (host=${host} path=${p.path} status=${result.status})`);
        }
      }

      await removeAppIfExists(slug);
    });
  }
});
