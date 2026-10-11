import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { startWebhookReceiver } from '../helpers/webhook.js';
import { sqliteQuery, sqliteExec, getAppId } from '../helpers/db.js';
import { deleteAppMetrics, startMetricRefresher } from '../helpers/metrics.js';
import { createBackupConfig } from '../helpers/backups.js';

const UI_HOOK = 'E2E Test Hook';

async function listWebhooks() {
  const res = await apiRequest('GET', '/api/webhooks');
  expect(res.status).toBe(200);
  return res.data;
}

async function listRules() {
  const res = await apiRequest('GET', '/api/alerts/rules');
  expect(res.status).toBe(200);
  return res.data;
}

async function createWebhook(body) {
  const res = await apiRequest('POST', '/api/webhooks', body);
  expect(res.status, `webhook create: ${JSON.stringify(res.data)}`).toBe(201);
  return res.data.id;
}

async function createRule(body) {
  const res = await apiRequest('POST', '/api/alerts/rules', { operator: '>', enabled: true, ...body });
  expect(res.status, `rule create: ${JSON.stringify(res.data)}`).toBe(201);
  return res.data.id;
}

// deleteRule drops the rule's alert history first: deleting a rule keeps its
// history rows (rule_id set to NULL).
async function deleteRule(id) {
  sqliteExec(`DELETE FROM alert_history WHERE rule_id=${id};`);
  const res = await apiRequest('DELETE', `/api/alerts/rules/${id}`);
  expect(res.status, `delete rule ${id}`).toBe(200);
}

test.describe('Alerts & Webhooks', () => {
  test.describe.configure({ mode: 'serial' });

  let receiver;

  test.beforeAll(async () => {
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.status).toBe(200);
    receiver = await startWebhookReceiver();
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    // Leftovers from a failed test; deleting the webhook cascades to its rules.
    for (const w of (await listWebhooks()).filter((w) => w.name === UI_HOOK)) {
      await apiRequest('DELETE', `/api/webhooks/${w.id}`);
    }
    await receiver.stop();
  });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/alerts`);
  });

  test('create webhook, with a test send from the form', async ({ page }) => {
    const url = `${receiver.url}/ui`;
    await page.getByRole('button', { name: 'Add Webhook' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByPlaceholder('My Webhook').fill(UI_HOOK);
    await dialog.locator('select').first().selectOption('custom');
    await dialog.getByPlaceholder('https://example.com/webhook').fill(url);

    await dialog.getByRole('button', { name: 'Send Test' }).click();
    const hit = await receiver.waitFor((r) => r.path === '/ui', 10_000);
    expect(hit.body).toMatchObject({ app: 'my-app', metric: 'cpu_pct', status: 'firing' });

    await dialog.getByRole('button', { name: 'Create' }).click();
    await expect(dialog).toBeHidden();

    const hook = (await listWebhooks()).find((w) => w.name === UI_HOOK);
    expect(hook).toMatchObject({ type: 'custom', url });
    await expect(page.locator('main table').first().getByRole('row', { name: new RegExp(UI_HOOK) })).toBeVisible();
  });

  test('create alert rule', async ({ page }) => {
    const hook = (await listWebhooks()).find((w) => w.name === UI_HOOK);
    expect(hook, 'webhook from the previous test').toBeTruthy();

    await page.getByRole('button', { name: 'Add Rule' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    const select = (optionText) => dialog.locator('select', { has: page.locator('option', { hasText: optionText }) });
    await select('All Apps').selectOption('e2e-nginx');
    await select('CPU Usage').selectOption('cpu_pct');
    await select('above').selectOption('>');
    await dialog.locator('input[type="number"]').first().fill('90');
    await select('Select webhook').selectOption(String(hook.id));
    await dialog.getByRole('button', { name: 'Create' }).click();
    await expect(dialog).toBeHidden();

    const rule = (await listRules()).find((r) => r.webhook_id === hook.id);
    // The rule must keep the app picked in the form, not become an all-apps rule.
    expect(rule).toMatchObject({ metric: 'cpu_pct', operator: '>', threshold: 90, enabled: true, app_slug: 'e2e-nginx' });
    await expect(page.locator('main table').nth(1).getByRole('row', { name: /CPU Usage > 90\.0%/ })).toBeVisible();
  });

  test('delete alert rule', async ({ page }) => {
    const hook = (await listWebhooks()).find((w) => w.name === UI_HOOK);
    expect(hook, 'webhook from the earlier test').toBeTruthy();

    const row = page.locator('main table').nth(1).getByRole('row', { name: /CPU Usage > 90\.0%/ });
    await row.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Confirm' }).click();

    await expect(row).toHaveCount(0);
    expect((await listRules()).filter((r) => r.webhook_id === hook.id)).toEqual([]);
  });

  test('delete webhook', async ({ page }) => {
    const row = page.locator('main table').first().getByRole('row', { name: new RegExp(UI_HOOK) });
    await row.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Confirm' }).click();

    await expect(row).toHaveCount(0);
    expect((await listWebhooks()).some((w) => w.name === UI_HOOK)).toBe(false);
  });
});

// Functional dispatch: the real evaluator + webhook dispatcher against a
// local receiver, driven by synthetic metric points. The evaluator ticks
// every 2s in e2e and walks enabled rules in id order, sending each webhook
// synchronously. So once a rule created later (higher id) has fired, every
// earlier rule was evaluated in that same tick; probe/control rules use this
// to prove that something did NOT fire without sleeping.
test.describe('Alerts - Functional Dispatch', () => {
  test.describe.configure({ mode: 'serial' });

  const EVAL_WAIT_MS = 20_000;
  const APP_SLUG = 'e2e-nginx';
  const DURATION_SEC = 60;

  let receiver;
  let appId;
  let webhookId;
  let ruleId;
  const ruleIds = [];
  const webhookIds = [];
  const backupConfigIds = [];

  const cpuEvents = (path, status) =>
    receiver.onPath(path).filter((r) => r.body.metric === 'cpu_pct' && r.body.status === status);

  // fireProbe adds a rule identical to the main one on its own webhook path
  // and waits for it to fire.
  async function fireProbe(path, refresher) {
    const whId = await createWebhook({ name: `E2E ${path}`, type: 'custom', url: `${receiver.url}${path}` });
    webhookIds.push(whId);
    const id = await createRule({ app_id: appId, metric: 'cpu_pct', threshold: 80, duration_sec: DURATION_SEC, webhook_id: whId });
    ruleIds.push(id);
    await receiver.waitFor((r) => r.path === path && r.body.status === 'firing', EVAL_WAIT_MS, refresher.lastError);
    return id;
  }

  test.beforeAll(async () => {
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.status).toBe(200);
    receiver = await startWebhookReceiver();
    appId = getAppId(APP_SLUG);

    // Stop the app so the real collector does not write points for it.
    const stop = await apiRequest('POST', `/api/apps/${APP_SLUG}/stop`);
    expect(stop.status).toBe(200);
    deleteAppMetrics(APP_SLUG);

    webhookId = await createWebhook({ name: 'E2E Dispatch Hook', type: 'custom', url: receiver.url });
    webhookIds.push(webhookId);
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    try {
      for (const id of [ruleId, ...ruleIds].filter(Boolean)) await deleteRule(id);
      for (const id of webhookIds) await apiRequest('DELETE', `/api/webhooks/${id}`);
      for (const id of backupConfigIds) await apiRequest('DELETE', `/api/backups/configs/${id}`);
      deleteAppMetrics(APP_SLUG);
      await receiver.stop();
    } finally {
      const start = await apiRequest('POST', `/api/apps/${APP_SLUG}/start`);
      expect(start.status).toBe(200);
      await waitForAppStatus(APP_SLUG, 'running', 60_000);
    }
  });

  test('CPU threshold breach fires webhook once while it persists', async () => {
    ruleId = await createRule({ app_id: appId, metric: 'cpu_pct', threshold: 80, duration_sec: DURATION_SEC, webhook_id: webhookId });

    receiver.clear();
    const refresher = startMetricRefresher(APP_SLUG, { high: true });
    try {
      const hit = await receiver.waitFor(
        (r) => r.path === '/' && r.body.status === 'firing' && r.body.metric === 'cpu_pct',
        EVAL_WAIT_MS,
        refresher.lastError,
      );
      expect(Object.keys(hit.body).sort()).toEqual(['app', 'metric', 'status', 'threshold', 'value']);
      expect(hit.body).toMatchObject({ app: APP_SLUG, metric: 'cpu_pct', status: 'firing', threshold: 80 });
      expect(hit.body.value).toBeGreaterThanOrEqual(80);

      // The history row is written right after the webhook is sent.
      await expect
        .poll(() => sqliteQuery(`SELECT id FROM alert_history WHERE rule_id=${ruleId} AND resolved_at IS NULL;`).length)
        .toBe(1);

      // Two later ticks (one per probe) must not fire the main rule again.
      await fireProbe('/probe1', refresher);
      await fireProbe('/probe2', refresher);
      expect(cpuEvents('/', 'firing'), 'main rule fired again while still firing').toHaveLength(1);
    } finally {
      refresher.stop();
    }
    for (const id of ruleIds.splice(0)) await deleteRule(id);
  });

  test('alert resolves when metrics drop below threshold', async () => {
    receiver.clear();
    const refresher = startMetricRefresher(APP_SLUG, { high: false });
    try {
      const hit = await receiver.waitFor(
        (r) => r.path === '/' && r.body.status === 'resolved' && r.body.metric === 'cpu_pct',
        EVAL_WAIT_MS,
        refresher.lastError,
      );
      expect(hit.body).toMatchObject({ app: APP_SLUG, metric: 'cpu_pct', status: 'resolved' });
    } finally {
      refresher.stop();
    }

    const hist = sqliteQuery(`SELECT resolved_at FROM alert_history WHERE rule_id=${ruleId};`);
    expect(hist).toHaveLength(1);
    expect(hist[0].resolved_at).not.toBeNull();
  });

  test('disabled rule does not fire', async () => {
    const upd = await apiRequest('PUT', `/api/alerts/rules/${ruleId}`, {
      app_id: appId,
      metric: 'cpu_pct',
      operator: '>',
      threshold: 80,
      duration_sec: DURATION_SEC,
      webhook_id: webhookId,
      enabled: false,
    });
    expect(upd.status).toBe(200);
    expect((await listRules()).find((r) => r.id === ruleId).enabled).toBe(false);
    const open = sqliteQuery(`SELECT id FROM alert_history WHERE rule_id=${ruleId} AND resolved_at IS NULL;`);
    expect(open, 'main rule must have no open alert, or a non-fire proves nothing').toEqual([]);

    receiver.clear();
    const refresher = startMetricRefresher(APP_SLUG, { high: true });
    try {
      // The control rule (higher id) firing means the disabled rule's turn
      // in that tick has passed.
      await fireProbe('/control', refresher);
      expect(cpuEvents('/', 'firing'), 'disabled rule fired').toEqual([]);
    } finally {
      refresher.stop();
    }
    for (const id of ruleIds.splice(0)) await deleteRule(id);
  });

  test('backup_failed alert fires for the failing app only', async () => {
    // A failed-backup rule scoped to another app, created first so its turn
    // comes before the nginx rule's.
    const otherHook = await createWebhook({ name: 'E2E Other App Hook', type: 'custom', url: `${receiver.url}/other` });
    webhookIds.push(otherHook);
    ruleIds.push(await createRule({ app_id: getAppId('e2e-postgres'), metric: 'backup_failed', threshold: 0, duration_sec: 0, webhook_id: otherHook }));
    ruleIds.push(await createRule({ app_id: appId, metric: 'backup_failed', threshold: 0, duration_sec: 0, webhook_id: webhookId }));

    // nginx is stopped, so the dump's docker exec fails.
    const cfgId = await createBackupConfig(APP_SLUG, { strategy: 'postgres', schedule_cron: '' });
    backupConfigIds.push(cfgId);

    receiver.clear();
    const trig = await apiRequest('POST', `/api/backups/configs/${cfgId}/run`);
    expect(trig.status).toBe(202);

    const hit = await receiver.waitFor((r) => r.path === '/' && r.body.metric === 'backup_failed', 60_000);
    expect(hit.body).toMatchObject({ app: APP_SLUG, metric: 'backup_failed', status: 'firing' });
    expect(receiver.onPath('/other'), 'rule scoped to e2e-postgres fired for e2e-nginx').toEqual([]);
  });

  test('backup_success alert names the app and says it succeeded', async () => {
    const slackHook = await createWebhook({ name: 'E2E Slack Hook', type: 'slack', url: `${receiver.url}/slack` });
    webhookIds.push(slackHook);
    ruleIds.push(await createRule({ app_id: getAppId('e2e-postgres'), metric: 'backup_success', threshold: 0, duration_sec: 0, webhook_id: slackHook }));

    const cfgId = await createBackupConfig('e2e-postgres', { strategy: 'postgres', schedule_cron: '', retention_count: 3 });
    backupConfigIds.push(cfgId);

    receiver.clear();
    const trig = await apiRequest('POST', `/api/backups/configs/${cfgId}/run`);
    expect(trig.status).toBe(202);

    const hit = await receiver.waitFor((r) => r.path === '/slack', 90_000);
    expect(hit.body.text).toContain('[firing] e2e-postgres');
    expect(hit.body.text).toContain('Backup Succeeded');
    expect(hit.body.text).not.toMatch(/failed/i);
  });
});
