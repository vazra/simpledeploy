import { test, expect } from '@playwright/test';
import { TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { startWebhookReceiver } from '../helpers/webhook.js';
import {
  sqliteExec,
  injectHighCPUWindow,
  insertMetricPoint,
  getAppId,
  replaceMetricsAtomic,
} from '../helpers/db.js';

// -----------------------------------------------------------------------------
// Webhook payload-format tests: verify the dispatcher produces the correct
// JSON body shape for each integration type (slack/telegram/discord/custom)
// plus a template_override sanity check, by injecting synthetic CPU metrics
// and inspecting the bytes received at a local HTTP listener.
// -----------------------------------------------------------------------------

test.describe('Alerts - Webhook Payload Formats', () => {
  test.describe.configure({ mode: 'serial' });

  // e2e server runs the evaluator every 2s (SIMPLEDEPLOY_ALERT_EVAL_INTERVAL).
  // A handful of cycles is plenty; 20s is a generous ceiling.
  const EVAL_WAIT_MS = 20_000;
  const APP_SLUG = 'e2e-nginx';
  const DURATION_SEC = 60;
  const THRESHOLD = 80;
  const INJECTED_CPU = 95;

  let receiver;
  let nginxWasRunning = false;
  const createdWebhookIds = [];
  const createdRuleIds = [];

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

  // Continuously re-inject high/low CPU points so the evaluator always sees a
  // fresh breach window. Atomic DELETE+INSERT so the faster e2e evaluator
  // tick (2s) can't see an empty window and flip firing->resolve->refire.
  function startMetricRefresher(slug, high) {
    let stopped = false;
    const tick = () => {
      if (stopped) return;
      try {
        replaceMetricsAtomic(slug, {
          cpu: high ? INJECTED_CPU : 5,
          memoryMb: high ? 100 : 50,
        });
      } catch (_) { /* transient lock errors ignored */ }
    };
    tick();
    const handle = setInterval(tick, 5_000);
    return () => { stopped = true; clearInterval(handle); };
  }

  async function createWebhook(body) {
    const res = await apiRequest('POST', '/api/webhooks', body);
    expect(res.ok, `webhook create failed: ${JSON.stringify(res)}`).toBeTruthy();
    createdWebhookIds.push(res.data.id);
    return res.data.id;
  }

  async function createRule(webhookId) {
    const appId = getAppId(APP_SLUG);
    const res = await apiRequest('POST', '/api/alerts/rules', {
      app_id: appId,
      metric: 'cpu_pct',
      operator: '>',
      threshold: THRESHOLD,
      duration_sec: DURATION_SEC,
      webhook_id: webhookId,
      enabled: true,
    });
    expect(res.ok, `rule create failed: ${JSON.stringify(res)}`).toBeTruthy();
    createdRuleIds.push(res.data.id);
    return res.data.id;
  }

  // Wait for a firing payload, then resolve it so a later test for the same
  // rule starts from a clean (non-firing) state.
  async function waitForFiring(predicate = () => true) {
    return receiver.waitFor(
      (r) => r && r.body && predicate(r),
      EVAL_WAIT_MS,
    );
  }

  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    receiver = await startWebhookReceiver();

    // Stop the nginx app so the real collector doesn't clobber injected points.
    const appRes = await apiRequest('GET', `/api/apps/${APP_SLUG}`);
    nginxWasRunning = appRes.ok && (appRes.data.Status || appRes.data.status) === 'running';
    if (nginxWasRunning) {
      await apiRequest('POST', `/api/apps/${APP_SLUG}/stop`);
      await new Promise((r) => setTimeout(r, 3_000));
    }
    deleteAppMetrics(APP_SLUG);
  });

  test.afterAll(async () => {
    try {
      for (const id of createdRuleIds) {
        await apiRequest('DELETE', `/api/alerts/rules/${id}`);
        sqliteExec(`DELETE FROM alert_history WHERE rule_id=${id};`);
      }
      for (const id of createdWebhookIds) {
        await apiRequest('DELETE', `/api/webhooks/${id}`);
      }
      deleteAppMetrics(APP_SLUG);
    } catch (_) { /* best-effort */ }

    if (receiver) await receiver.stop();

    if (nginxWasRunning) {
      try { await apiRequest('POST', `/api/apps/${APP_SLUG}/start`); } catch (_) {}
    }
  });

  // ---- Slack --------------------------------------------------------------
  test('slack webhook payload shape', async () => {
    test.setTimeout(120_000);

    const whId = await createWebhook({
      name: 'E2E Slack Hook',
      type: 'slack',
      url: receiver.url,
    });
    const ruleId = await createRule(whId);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      const hit = await waitForFiring((r) => r.body && typeof r.body.text === 'string' && r.body.text.includes('[firing]'));

      // Headers + JSON parse already done by receiver.
      expect(hit.headers['content-type']).toMatch(/application\/json/i);

      // Schema: exactly { text }.
      expect(Object.keys(hit.body).sort()).toEqual(['text']);
      expect(typeof hit.body.text).toBe('string');

      // Content: "[firing] <app> - CPU > 80.0% (current: <val>%)"
      const text = hit.body.text;
      expect(text).toMatch(/^\[firing\]/);
      expect(text).toContain(APP_SLUG);
      expect(text).toContain('CPU');
      expect(text).toContain('>');
      expect(text).toContain('80.0%');
      const m = text.match(/current:\s*([\d.]+)%/);
      expect(m, `expected "current: N%" in ${text}`).toBeTruthy();
      const current = Number(m[1]);
      expect(current).toBeGreaterThanOrEqual(THRESHOLD);
      expect(current).toBeLessThanOrEqual(100);
    } finally {
      stopRefresh();
    }

    // Resolve to clean up for next test.
    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopLow = startMetricRefresher(APP_SLUG, false);
    try {
      await receiver.waitFor(
        (r) => r.body && r.body.text && r.body.text.includes('[resolved]'),
        EVAL_WAIT_MS,
      );
    } finally {
      stopLow();
    }
    // Delete the rule so its history row doesn't interfere with later rules.
    await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
  });

  // ---- Slack: firing -> resolved transition ------------------------------
  test('slack payload flips status firing -> resolved', async () => {
    test.setTimeout(180_000);

    const whId = await createWebhook({
      name: 'E2E Slack Transition Hook',
      type: 'slack',
      url: receiver.url,
    });
    const ruleId = await createRule(whId);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);

    // Phase 1: drive firing.
    let stopRefresh = startMetricRefresher(APP_SLUG, true);
    try {
      const firing = await waitForFiring(
        (r) => r.body && r.body.text && r.body.text.startsWith('[firing]'),
      );
      expect(firing.body.text).toMatch(/^\[firing\]/);
    } finally {
      stopRefresh();
    }

    // Phase 2: drop metrics below threshold, expect resolved payload.
    deleteAppMetrics(APP_SLUG);
    stopRefresh = startMetricRefresher(APP_SLUG, false);
    try {
      const resolved = await receiver.waitFor(
        (r) => r.body && r.body.text && r.body.text.startsWith('[resolved]'),
        EVAL_WAIT_MS,
      );
      // Still exactly { text }, just with resolved status.
      expect(Object.keys(resolved.body).sort()).toEqual(['text']);
      expect(resolved.body.text).toContain(APP_SLUG);
      expect(resolved.body.text).toContain('CPU');
    } finally {
      stopRefresh();
    }

    await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
  });

  // ---- Telegram -----------------------------------------------------------
  test('telegram webhook payload shape', async () => {
    test.setTimeout(120_000);

    const whId = await createWebhook({
      name: 'E2E Telegram Hook',
      type: 'telegram',
      url: receiver.url,
    });
    const ruleId = await createRule(whId);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      const hit = await waitForFiring(
        (r) => r.body && r.body.parse_mode === 'HTML' && typeof r.body.text === 'string',
      );

      expect(hit.headers['content-type']).toMatch(/application\/json/i);

      // Schema: exactly { text, parse_mode }.
      expect(Object.keys(hit.body).sort()).toEqual(['parse_mode', 'text']);
      expect(hit.body.parse_mode).toBe('HTML');
      expect(typeof hit.body.text).toBe('string');

      // Content: "[firing] <app>\nCPU > 80.0% (current: N%)"
      const text = hit.body.text;
      expect(text).toMatch(/^\[firing\]/);
      expect(text).toContain(APP_SLUG);
      expect(text).toContain('\n');
      expect(text).toContain('CPU');
      expect(text).toContain('> 80.0%');
      expect(text).toMatch(/current:\s*[\d.]+%/);
    } finally {
      stopRefresh();
    }

    // Resolve before next test.
    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopLow = startMetricRefresher(APP_SLUG, false);
    try {
      await receiver.waitFor(
        (r) => r.body && r.body.text && r.body.text.startsWith('[resolved]'),
        EVAL_WAIT_MS,
      );
    } finally {
      stopLow();
    }
    await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
  });

  // ---- Discord ------------------------------------------------------------
  test('discord webhook payload shape', async () => {
    test.setTimeout(120_000);

    const whId = await createWebhook({
      name: 'E2E Discord Hook',
      type: 'discord',
      url: receiver.url,
    });
    const ruleId = await createRule(whId);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      const hit = await waitForFiring(
        (r) => r.body && typeof r.body.content === 'string' && r.body.content.startsWith('[firing]'),
      );

      expect(hit.headers['content-type']).toMatch(/application\/json/i);

      // Schema: exactly { content }.
      expect(Object.keys(hit.body).sort()).toEqual(['content']);
      expect(typeof hit.body.content).toBe('string');

      const content = hit.body.content;
      expect(content).toMatch(/^\[firing\]/);
      expect(content).toContain(APP_SLUG);
      expect(content).toContain('CPU');
      expect(content).toContain('> 80.0%');
      expect(content).toMatch(/current:\s*[\d.]+%/);
    } finally {
      stopRefresh();
    }

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopLow = startMetricRefresher(APP_SLUG, false);
    try {
      await receiver.waitFor(
        (r) => r.body && r.body.content && r.body.content.startsWith('[resolved]'),
        EVAL_WAIT_MS,
      );
    } finally {
      stopLow();
    }
    await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
  });

  // ---- Custom (default template) ------------------------------------------
  test('custom webhook default payload shape', async () => {
    test.setTimeout(120_000);

    const whId = await createWebhook({
      name: 'E2E Custom Hook',
      type: 'custom',
      url: receiver.url,
    });
    const ruleId = await createRule(whId);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      const hit = await waitForFiring(
        (r) => r.body && r.body.status === 'firing' && r.body.metric === 'cpu_pct',
      );

      expect(hit.headers['content-type']).toMatch(/application\/json/i);

      // Schema: exactly { app, metric, value, threshold, status }.
      expect(Object.keys(hit.body).sort()).toEqual(
        ['app', 'metric', 'status', 'threshold', 'value'],
      );
      expect(hit.body.app).toBe(APP_SLUG);
      expect(hit.body.metric).toBe('cpu_pct');
      expect(hit.body.status).toBe('firing');
      expect(typeof hit.body.value).toBe('number');
      expect(typeof hit.body.threshold).toBe('number');
      expect(hit.body.threshold).toBeCloseTo(THRESHOLD, 2);
      expect(hit.body.value).toBeGreaterThanOrEqual(THRESHOLD);
      expect(hit.body.value).toBeLessThanOrEqual(100);
    } finally {
      stopRefresh();
    }

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopLow = startMetricRefresher(APP_SLUG, false);
    try {
      await receiver.waitFor(
        (r) => r.body && r.body.status === 'resolved' && r.body.metric === 'cpu_pct',
        EVAL_WAIT_MS,
      );
    } finally {
      stopLow();
    }
    await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
  });

  // ---- template_override --------------------------------------------------
  test('template_override replaces default payload', async () => {
    test.setTimeout(120_000);

    const override = '{"custom":"tpl","metric":"{{.Metric}}","value":{{.Value}}}';
    const whId = await createWebhook({
      name: 'E2E Override Hook',
      type: 'custom',
      url: receiver.url,
      template_override: override,
    });
    const ruleId = await createRule(whId);

    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopRefresh = startMetricRefresher(APP_SLUG, true);

    try {
      const hit = await waitForFiring(
        (r) => r.body && r.body.custom === 'tpl' && r.body.metric === 'cpu_pct',
      );

      expect(hit.headers['content-type']).toMatch(/application\/json/i);

      // Only the three override fields should be present. The default
      // custom fields (app, threshold, status) must NOT appear.
      expect(Object.keys(hit.body).sort()).toEqual(['custom', 'metric', 'value']);
      expect(hit.body.custom).toBe('tpl');
      expect(hit.body.metric).toBe('cpu_pct');
      expect(typeof hit.body.value).toBe('number');
      expect(hit.body.value).toBeGreaterThanOrEqual(THRESHOLD);
    } finally {
      stopRefresh();
    }

    // Resolve.
    receiver.clear();
    deleteAppMetrics(APP_SLUG);
    const stopLow = startMetricRefresher(APP_SLUG, false);
    try {
      // Resolved renders with the override too; just wait a full cycle and move on.
      await receiver.waitFor(
        (r) => r.body && r.body.custom === 'tpl',
        EVAL_WAIT_MS,
      ).catch(() => { /* override doesn't surface status; best-effort */ });
    } finally {
      stopLow();
    }
    await apiRequest('DELETE', `/api/alerts/rules/${ruleId}`);
  });
});
