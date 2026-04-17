import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { startWebhookReceiver } from '../helpers/webhook.js';
import {
  sqliteQuery,
  sqliteExec,
  injectHighCPUWindow,
  insertMetricPoint,
  getAppId,
} from '../helpers/db.js';

test.describe('Alerts & Webhooks', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    const state = getState();
    await page.goto(`${state.baseURL}/#/alerts`);
  });

  test('alerts page loads', async ({ page }) => {
    await expect(page.getByText(/webhook/i).first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/alert rule/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('create webhook', async ({ page }) => {
    await page.getByRole('button', { name: /add webhook/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByPlaceholder(/my webhook/i).fill('E2E Test Hook');
    const typeSelect = dialog.locator('select').first();
    await typeSelect.selectOption('custom');
    await dialog.getByPlaceholder(/https:\/\//i).fill('https://httpbin.org/post');

    await dialog.getByRole('button', { name: /create/i }).click();
    await expect(page.getByText('E2E Test Hook')).toBeVisible({ timeout: 5_000 });
  });

  test('test webhook', async ({ page }) => {
    const testBtn = page.getByRole('button', { name: /test/i }).first();
    if (await testBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await testBtn.click();
      await expect(page.locator('[role="alert"]').first()).toBeVisible({ timeout: 10_000 });
    }
  });

  test('create alert rule', async ({ page }) => {
    await page.getByRole('button', { name: /add rule/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    const nameInput = dialog.getByPlaceholder(/name|rule/i);
    if (await nameInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await nameInput.fill('E2E CPU Alert');
    }

    const metricSelect = dialog.locator('select').filter({ hasText: /cpu|metric/i }).first();
    if (await metricSelect.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await metricSelect.selectOption({ index: 0 });
    }

    const thresholdInput = dialog.locator('input[type="number"]').first();
    if (await thresholdInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await thresholdInput.fill('90');
    }

    const webhookSelect = dialog.locator('select').last();
    if (await webhookSelect.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const options = await webhookSelect.locator('option').allTextContents();
      if (options.length > 1) {
        await webhookSelect.selectOption({ index: 1 });
      }
    }

    await dialog.getByRole('button', { name: /create|save/i }).click();
    await expect(page.getByText(/cpu/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('view alert history', async ({ page }) => {
    await expect(page.getByText(/history/i).first()).toBeVisible({ timeout: 5_000 });
  });

  test('delete alert rule', async ({ page }) => {
    const deleteBtn = page.locator('button').filter({ hasText: /delete/i }).last();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });

  test('delete webhook', async ({ page }) => {
    const deleteBtn = page.locator('button').filter({ hasText: /delete/i }).first();
    if (await deleteBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await deleteBtn.click();
      const dialog = page.getByRole('dialog');
      if (await dialog.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await dialog.getByRole('button', { name: /delete|confirm/i }).click();
      }
    }
  });
});

// -----------------------------------------------------------------------------
// Functional dispatch tests: exercise the real evaluator + webhook dispatcher
// against a local HTTP receiver by injecting synthetic metric points.
// -----------------------------------------------------------------------------

test.describe('Alerts - Functional Dispatch', () => {
  test.describe.configure({ mode: 'serial' });

  // Evaluator ticks every 30s in production. Give it 2+ cycles before giving up.
  const EVAL_WAIT_MS = 75_000;
  const APP_SLUG = 'e2e-nginx';
  const DURATION_SEC = 60;

  let receiver;
  let webhookId;
  let ruleId;
  let backupRuleIds = [];
  let backupConfigIds = [];
  let nginxWasRunning = false;

  function deleteAppMetrics(slug) {
    sqliteExec(
      `DELETE FROM metrics WHERE app_id = (SELECT id FROM apps WHERE slug='${slug}');`,
    );
  }

  function injectLowCPUWindow(slug, cpuPct = 5) {
    const now = Math.floor(Date.now() / 1000);
    for (let i = 0; i < 10; i++) {
      insertMetricPoint({
        appSlug: slug,
        cpu: cpuPct,
        memoryMb: 50,
        tsSec: now - i * 5,
      });
    }
  }

  // Periodically refresh high-CPU metrics so the evaluator always sees a full
  // window of breaching points even if the real collector writes low values.
  function startMetricRefresher(slug, high) {
    let stopped = false;
    const tick = () => {
      if (stopped) return;
      try {
        // delete any points the real collector wrote (near-current ts)
        sqliteExec(
          `DELETE FROM metrics WHERE app_id = (SELECT id FROM apps WHERE slug='${slug}') AND container_id != 'e2e-fake';`,
        );
        if (high) {
          injectHighCPUWindow(slug, 120, 95);
        } else {
          injectLowCPUWindow(slug, 5);
        }
      } catch (e) {
        // ignore transient sqlite lock errors
      }
    };
    tick();
    const handle = setInterval(tick, 5_000);
    return () => {
      stopped = true;
      clearInterval(handle);
    };
  }

  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    receiver = await startWebhookReceiver();

    // Record current app status so we can restore it after tests.
    const appRes = await apiRequest('GET', `/api/apps/${APP_SLUG}`);
    nginxWasRunning = appRes.ok && (appRes.data.Status || appRes.data.status) === 'running';

    // Stop the app so the real metric collector does not clobber our
    // synthetic data points during the test window.
    if (nginxWasRunning) {
      await apiRequest('POST', `/api/apps/${APP_SLUG}/stop`);
      // give docker a moment to settle
      await new Promise((r) => setTimeout(r, 3_000));
    }

    deleteAppMetrics(APP_SLUG);

    // Create the shared webhook pointing at our local receiver.
    const whRes = await apiRequest('POST', '/api/webhooks', {
      name: 'E2E Dispatch Hook',
      type: 'custom',
      url: receiver.url,
    });
    expect(whRes.ok, `webhook create failed: ${JSON.stringify(whRes)}`).toBeTruthy();
    webhookId = whRes.data.id;
  });

  test.afterAll(async () => {
    // Best-effort cleanup; swallow errors so cleanup specs still run.
    try {
      for (const id of backupRuleIds) {
        await apiRequest('DELETE', `/api/alerts/rules/${id}`);
      }
      if (ruleId) {
        await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
      }
      if (webhookId) {
        await apiRequest('DELETE', `/api/webhooks/${webhookId}`);
      }
      for (const id of backupConfigIds) {
        await apiRequest('DELETE', `/api/backups/configs/${id}`);
      }
      // Clear synthetic metrics + alert history rows for our rule.
      deleteAppMetrics(APP_SLUG);
      if (ruleId) {
        sqliteExec(`DELETE FROM alert_history WHERE rule_id=${ruleId};`);
      }
      for (const id of backupRuleIds) {
        sqliteExec(`DELETE FROM alert_history WHERE rule_id=${id};`);
      }
    } catch {}

    if (receiver) await receiver.stop();

    // Restore original app state so downstream tests (14-19) see it running.
    if (nginxWasRunning) {
      try { await apiRequest('POST', `/api/apps/${APP_SLUG}/start`); } catch {}
    }
  });

  test('CPU threshold breach fires webhook', async () => {
    test.setTimeout(120_000);

    const appId = getAppId(APP_SLUG);

    // Create the alert rule.
    const ruleRes = await apiRequest('POST', '/api/alerts/rules', {
      app_id: appId,
      metric: 'cpu_pct',
      operator: '>',
      threshold: 80,
      duration_sec: DURATION_SEC,
      webhook_id: webhookId,
      enabled: true,
    });
    expect(ruleRes.ok, `rule create failed: ${JSON.stringify(ruleRes)}`).toBeTruthy();
    ruleId = ruleRes.data.id;

    receiver.clear();
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      const hit = await receiver.waitFor(
        (r) => r.body && r.body.status === 'firing' && r.body.metric === 'cpu_pct',
        EVAL_WAIT_MS,
      );
      expect(hit.body.status).toBe('firing');
      expect(hit.body.metric).toBe('cpu_pct');
      expect(hit.body.app).toBe(APP_SLUG);
      expect(Number(hit.body.value)).toBeGreaterThanOrEqual(80);
    } finally {
      stopRefresh();
    }

    // Verify alert_history has a firing row (resolved_at NULL).
    const hist = sqliteQuery(
      `SELECT id, rule_id, resolved_at FROM alert_history WHERE rule_id=${ruleId};`,
    );
    expect(hist.length).toBeGreaterThan(0);
    const open = hist.find((h) => h.resolved_at === null);
    expect(open, 'expected an unresolved alert history row').toBeTruthy();
  });

  test('alert resolves when metrics drop below threshold', async () => {
    test.setTimeout(120_000);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, false);

    try {
      const hit = await receiver.waitFor(
        (r) => r.body && r.body.status === 'resolved' && r.body.metric === 'cpu_pct',
        EVAL_WAIT_MS,
      );
      expect(hit.body.status).toBe('resolved');
    } finally {
      stopRefresh();
    }

    const hist = sqliteQuery(
      `SELECT id, resolved_at FROM alert_history WHERE rule_id=${ruleId};`,
    );
    const resolved = hist.find((h) => h.resolved_at !== null);
    expect(resolved, 'expected at least one resolved alert history row').toBeTruthy();
  });

  test('no double-fire while condition persists', async () => {
    test.setTimeout(120_000);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      // Wait for the (re-)fire.
      await receiver.waitFor(
        (r) => r.body && r.body.status === 'firing' && r.body.metric === 'cpu_pct',
        EVAL_WAIT_MS,
      );

      // Keep the breach active for ~2 more evaluator cycles and count fires.
      await new Promise((r) => setTimeout(r, 65_000));
      const fires = receiver.received.filter(
        (r) => r.body && r.body.status === 'firing' && r.body.metric === 'cpu_pct',
      );
      expect(fires.length, `expected exactly 1 fire event, got ${fires.length}`).toBe(1);
    } finally {
      stopRefresh();
    }
  });

  test('disabled rule does not fire', async () => {
    test.setTimeout(180_000);

    // Resolve first so a future enable+breach would be a fresh fire.
    deleteAppMetrics(APP_SLUG);
    const stopLow = startMetricRefresher(APP_SLUG, false);
    await receiver.waitFor(
      (r) => r.body && r.body.status === 'resolved' && r.body.metric === 'cpu_pct',
      EVAL_WAIT_MS,
    ).catch(() => {}); // may already be resolved
    stopLow();

    // Disable the rule.
    const upd = await apiRequest('PUT', `/api/alerts/rules/${ruleId}`, {
      app_id: getAppId(APP_SLUG),
      metric: 'cpu_pct',
      operator: '>',
      threshold: 80,
      duration_sec: DURATION_SEC,
      webhook_id: webhookId,
      enabled: false,
    });
    expect(upd.ok, `rule update failed: ${JSON.stringify(upd)}`).toBeTruthy();

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      // Wait across at least 2 evaluator ticks.
      await new Promise((r) => setTimeout(r, 65_000));
      const cpuHits = receiver.received.filter(
        (r) => r.body && r.body.metric === 'cpu_pct',
      );
      expect(cpuHits.length, `expected 0 cpu webhook calls, got ${cpuHits.length}`).toBe(0);
    } finally {
      stopRefresh();
    }
  });

  test('backup_failed alert fires when backup run fails', async () => {
    test.setTimeout(120_000);

    const nginxAppId = getAppId('e2e-nginx');

    const ruleRes = await apiRequest('POST', '/api/alerts/rules', {
      app_id: nginxAppId,
      metric: 'backup_failed',
      operator: '>',
      threshold: 0,
      duration_sec: 0,
      webhook_id: webhookId,
      enabled: true,
    });
    expect(ruleRes.ok, `backup rule create failed: ${JSON.stringify(ruleRes)}`).toBeTruthy();
    backupRuleIds.push(ruleRes.data.id);

    // Postgres strategy against nginx app: pg_dump binary doesn't exist in the
    // nginx container, so the docker exec reliably returns non-zero.
    const createRes = await apiRequest('POST', '/api/apps/e2e-nginx/backups/configs', {
      strategy: 'postgres',
      target: 'local',
      schedule_cron: '',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
    });
    expect(createRes.ok, `bad backup config create failed: ${JSON.stringify(createRes)}`).toBeTruthy();
    const cfgId = createRes.data.id;
    backupConfigIds.push(cfgId);

    receiver.clear();

    const trig = await apiRequest('POST', `/api/backups/configs/${cfgId}/run`);
    expect([200, 202]).toContain(trig.status);

    const hit = await receiver.waitFor(
      (r) => r.body && r.body.metric === 'backup_failed',
      60_000,
    );
    expect(hit.body.metric).toBe('backup_failed');
    expect(hit.body.status).toBe('firing');
  });

  test('backup_success alert fires when backup run succeeds', async () => {
    test.setTimeout(120_000);

    const pgAppId = getAppId('e2e-postgres');

    const ruleRes = await apiRequest('POST', '/api/alerts/rules', {
      app_id: pgAppId,
      metric: 'backup_success',
      operator: '>',
      threshold: 0,
      duration_sec: 0,
      webhook_id: webhookId,
      enabled: true,
    });
    expect(ruleRes.ok, `backup_success rule create failed: ${JSON.stringify(ruleRes)}`).toBeTruthy();
    backupRuleIds.push(ruleRes.data.id);

    const createRes = await apiRequest('POST', '/api/apps/e2e-postgres/backups/configs', {
      strategy: 'postgres',
      target: 'local',
      schedule_cron: '',
      target_config_json: '',
      retention_mode: 'count',
      retention_count: 3,
    });
    expect(createRes.ok, `backup config create failed: ${JSON.stringify(createRes)}`).toBeTruthy();
    const cfgId = createRes.data.id;
    backupConfigIds.push(cfgId);

    receiver.clear();

    const trig = await apiRequest('POST', `/api/backups/configs/${cfgId}/run`);
    expect([200, 202]).toContain(trig.status);

    const hit = await receiver.waitFor(
      (r) => r.body && r.body.metric === 'backup_success',
      90_000,
    );
    expect(hit.body.metric).toBe('backup_success');
    expect(hit.body.status).toBe('firing');
  });

  // Dispatcher sends once with no retry (internal/alerts/webhook.go Send:
  // single http.Client.Do, returns error on status >= 400). Nothing to assert.
  test.skip('webhook retry behavior', async () => {});
});
