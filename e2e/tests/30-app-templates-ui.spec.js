// App templates (ui/src/lib/appTemplates.js) must render to compose files
// that both SimpleDeploy's validator and docker compose accept, or the
// wizard deploys something that fails. The full deploy matrix lives in
// templates-deploy-all.spec.js (E2E_TEMPLATES=1).

import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { appTemplates, applyVars, applyAccessMode, countEndpoints, generateSecret } from '../../ui/src/lib/appTemplates.js';

// Values as the template picker would fill them in Custom domain mode, with
// a distinct domain per domain variable.
function varsFor(tpl) {
  const vars = {};
  for (const v of tpl.variables || []) {
    if (v.type === 'domain') vars[v.key] = `${v.key.replace(/_/g, '-')}.${tpl.id}.e2e.test`;
    else if (v.default != null) vars[v.key] = v.default;
    else if (v.type === 'secret') vars[v.key] = generateSecret(v.generate?.length || 24, v.generate?.charset || 'base58');
    else if (v.type === 'email') vars[v.key] = 'e2e@example.com';
    else vars[v.key] = `e2e-${v.key}`;
  }
  return vars;
}

test.describe('App templates', () => {
  test(`all ${appTemplates.length} templates render to a compose file the server and docker compose accept`, async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    const failures = [];
    for (const tpl of appTemplates) {
      const compose = applyAccessMode(applyVars(tpl.compose, varsFor(tpl)), 'custom');
      // JSON is valid YAML, so the rendered tree is sent as-is.
      const text = JSON.stringify(compose);
      const res = await apiRequest('POST', '/api/apps/validate-compose', { compose: Buffer.from(text).toString('base64') });
      if (res.status !== 200 || res.data.valid !== true) {
        failures.push(`${tpl.id}: validate-compose ${res.status} ${JSON.stringify(res.data)}`);
      }
      // SimpleDeploy's parser skips compose consistency checks (e.g. a
      // service using an undeclared named volume); docker compose up does not.
      try {
        execFileSync('docker', ['compose', '-p', 'sd-e2e-template-check', '-f', '-', 'config', '--quiet'], {
          input: text, stdio: ['pipe', 'pipe', 'pipe'], encoding: 'utf-8',
        });
      } catch (e) {
        failures.push(`${tpl.id}: docker compose config: ${(e.stderr || e.message).trim()}`);
      }
    }
    expect(failures).toEqual([]);
  });

  // Quick test (the default access mode) gives every domain variable the
  // same sslip.io name; multi-endpoint templates must still deploy.
  test('multi-endpoint templates apply to a valid compose in the default Quick test mode', async ({ page }) => {
    const multi = appTemplates.filter((t) => countEndpoints(t.compose) > 1);
    expect(multi.length).toBeGreaterThan(0);
    await loginAsAdmin(page);
    const state = getState();
    for (const tpl of multi) {
      await page.goto(`${state.baseURL}/#/`);
      await page.reload();
      await page.getByRole('button', { name: 'Deploy App' }).first().click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: /^browse templates$/i }).click();
      await dialog.getByRole('button', { name: `Use template ${tpl.name}` }).click();

      const validated = page.waitForResponse((r) => r.url().endsWith('/api/apps/validate-compose'));
      await dialog.getByRole('button', { name: /^apply/i }).click();
      const res = await validated;
      expect(res.status()).toBe(200);
      expect(await res.json(), `${tpl.id} in Quick test mode`).toEqual({ valid: true });
      await expect(dialog.getByText('Valid compose file', { exact: true })).toBeVisible();
    }
  });
});
