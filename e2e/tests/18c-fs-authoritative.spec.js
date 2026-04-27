import { test, expect } from '@playwright/test';
import { readFileSync, writeFileSync, existsSync, readdirSync } from 'fs';
import { join } from 'path';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';

// Verifies FS-authoritative state: hand-editing the per-app sidecar file
// (simpledeploy.yml) propagates to the running DB via the watcher within a
// few seconds, so the rule shows up via API and on the Alerts page.
test.describe('FS-authoritative sidecar edits', () => {
  test('hand-edit simpledeploy.yml propagates via watcher', async ({ page }) => {
    test.setTimeout(60_000);

    const slug = 'e2e-nginx';
    const webhookName = 'fs-auth-test';
    const state = getState();
    const sidecarPath = join(state.appsDir, slug, 'simpledeploy.yml');

    await loginAsAdmin(page);
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);

    // Pre-create the named webhook so the watcher can resolve it when applying
    // the rule. If the webhook is missing the watcher logs "skipping" and the
    // rule is dropped.
    const whList = await apiRequest('GET', '/api/webhooks');
    const existing = (whList.data || []).find((w) => w.name === webhookName);
    let webhookId = existing ? (existing.id || existing.ID) : null;
    if (!webhookId) {
      const whRes = await apiRequest('POST', '/api/webhooks', {
        name: webhookName,
        type: 'custom',
        url: 'https://example.invalid/hook',
      });
      expect(whRes.ok, `webhook create failed: ${JSON.stringify(whRes)}`).toBeTruthy();
      webhookId = whRes.data.id;
    }

    // Bootstrap the sidecar by creating a per-app alert rule via API. App-level
    // mutations trigger configsync.ScheduleAppWrite which writes the sidecar
    // after a 500ms debounce. (UpsertApp itself does not fire the per-app hook,
    // so a freshly deployed app may not have a sidecar yet.)
    const appRes = await apiRequest('GET', `/api/apps/${slug}`);
    expect(appRes.ok, `get app failed: ${JSON.stringify(appRes)}`).toBeTruthy();
    const appId = appRes.data.id || appRes.data.ID;
    expect(appId, 'app id required').toBeTruthy();

    const seedRes = await apiRequest('POST', '/api/alerts/rules', {
      app_id: appId,
      metric: 'cpu_pct',
      operator: '>',
      threshold: 50,
      duration_sec: 60,
      webhook_id: webhookId,
      enabled: true,
    });
    expect(seedRes.ok, `seed alert rule failed: ${JSON.stringify(seedRes)}`).toBeTruthy();
    const seedRuleId = seedRes.data.id || seedRes.data.ID;

    // Wait for the sidecar to be written AND contain the seed rule. configsync
    // writes are debounced 500ms; we poll so we don't race a future write that
    // would clobber our edit.
    const sidecarDeadline = Date.now() + 15_000;
    let original = '';
    while (Date.now() < sidecarDeadline) {
      if (existsSync(sidecarPath)) {
        const c = readFileSync(sidecarPath, 'utf-8');
        if (c.includes('alert_rules:') && c.includes('threshold: 50')) {
          original = c;
          break;
        }
      }
      await page.waitForTimeout(500);
    }
    if (!original) {
      const appDir = join(state.appsDir, slug);
      const listing = existsSync(appDir) ? readdirSync(appDir).join(', ') : '<missing>';
      const cur = existsSync(sidecarPath) ? readFileSync(sidecarPath, 'utf-8') : '<missing>';
      throw new Error(`seed rule not in sidecar; dir: ${listing}; content:\n${cur}`);
    }
    // Pause to ensure no further configsync writes are pending which could
    // clobber our hand-edit.
    await page.waitForTimeout(2_000);
    expect(original, 'sidecar should have content').toContain('version:');

    // Append an alert_rules block (or extend if it already exists).
    const ruleBlock = [
      '  - metric: cpu_pct',
      '    operator: ">"',
      '    threshold: 99',
      '    duration_sec: 60',
      `    webhook: ${webhookName}`,
      '    enabled: true',
    ].join('\n');

    let next;
    if (/^alert_rules:\s*$/m.test(original) || /^alert_rules:\s*\n/.test(original)) {
      // Append to existing alert_rules list.
      next = original.replace(/^alert_rules:\s*\n/m, `alert_rules:\n${ruleBlock}\n`);
    } else {
      next = original.replace(/\s*$/, '\n') + `alert_rules:\n${ruleBlock}\n`;
    }

    // Direct in-place write (not tmp+rename): fsnotify on macOS may swallow
    // some rename-over-existing event sequences, so a plain Write event is
    // most reliable for triggering the watcher.
    writeFileSync(sidecarPath, next);

    // Wait for watcher debounce + apply, then poll API for the new rule.
    const deadline = Date.now() + 30_000;
    let found = null;
    while (Date.now() < deadline) {
      await page.waitForTimeout(1_000);
      const list = await apiRequest('GET', '/api/alerts/rules');
      if (list.ok) {
        const rules = list.data || [];
        found = rules.find((r) => {
          const slugMatch = (r.app_slug || r.AppSlug) === slug;
          const metric = r.metric || r.Metric;
          const threshold = r.threshold ?? r.Threshold;
          return slugMatch && metric === 'cpu_pct' && Number(threshold) === 99;
        });
        if (found) break;
      }
    }
    if (!found) {
      const cur = existsSync(sidecarPath) ? readFileSync(sidecarPath, 'utf-8') : '<missing>';
      throw new Error(
        `watcher did not apply hand-edited sidecar within timeout. sidecar:\n${cur}`,
      );
    }

    // UI sanity: Alerts page loads and renders rule rows. Rule text varies
    // (metric labels, app slug, threshold), so we just assert the page is up
    // and shows our threshold value somewhere in the main pane.
    await page.goto(`${state.baseURL}/#/alerts`);
    await expect(page.locator('main')).toBeVisible({ timeout: 10_000 });
    await expect(
      page.locator('main').getByText(/99/).first()
    ).toBeVisible({ timeout: 15_000 });

    // Cleanup: delete rules and webhook so later specs see a clean slate.
    // Restore the sidecar BEFORE deleting DB rows so the watcher does not
    // re-apply them; then delete via API which rewrites the sidecar.
    writeFileSync(sidecarPath, original);
    await page.waitForTimeout(2_000); // let watcher reconcile

    const ruleId = found.id || found.ID;
    if (ruleId) {
      await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
    }
    if (seedRuleId) {
      await apiRequest('DELETE', `/api/alerts/rules/${seedRuleId}`);
    }
    if (webhookId) {
      await apiRequest('DELETE', `/api/webhooks/${webhookId}`);
    }
  });
});
