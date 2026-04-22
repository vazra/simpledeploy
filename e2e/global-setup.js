import { buildBinary, startServer } from './helpers/server.js';
import { writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';
import { chromium, request } from '@playwright/test';
import { TEST_ADMIN } from './helpers/auth.js';

const STATE_FILE = join(import.meta.dirname, '.e2e-state.json');
const AUTH_DIR = join(import.meta.dirname, '.auth');
const STORAGE_STATE = join(AUTH_DIR, 'admin.json');

export default async function globalSetup() {
  const binPath = await buildBinary();
  const server = await startServer(binPath);

  process.on('exit', () => {
    try { server.proc.kill('SIGTERM'); } catch {}
  });

  const state = {
    pid: server.proc.pid,
    port: server.port,
    proxyPort: server.proxyPort,
    dataDir: server.dataDir,
    appsDir: server.appsDir,
    configPath: server.configPath,
    logPath: server.logPath,
    baseURL: server.baseURL,
    proxyURL: server.proxyURL,
  };
  writeFileSync(STATE_FILE, JSON.stringify(state));
  process.env.SIMPLEDEPLOY_PORT = String(server.port);

  // Templates-only mode: 01-setup.spec.js is excluded (no-admin-yet
  // negative tests would fail after we pre-create the admin). Create the
  // admin via API + save a logged-in storageState so every shard's tests
  // can skip per-test UI login.
  if (process.env.E2E_TEMPLATES === '1') {
    const req = await request.newContext({ baseURL: server.baseURL });
    const res = await req.post('/api/setup', {
      data: {
        username: TEST_ADMIN.username,
        password: TEST_ADMIN.password,
        display_name: TEST_ADMIN.displayName,
        email: TEST_ADMIN.email,
      },
    });
    if (!res.ok() && res.status() !== 409) {
      throw new Error(`setup failed: ${res.status()} ${await res.text()}`);
    }
    await req.dispose();

    mkdirSync(AUTH_DIR, { recursive: true });
    const browser = await chromium.launch();
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto(`${server.baseURL}/#/login`);
    await page.waitForSelector('#username', { timeout: 10_000 });
    await page.locator('#username').fill(TEST_ADMIN.username);
    await page.locator('#password').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForSelector('aside', { timeout: 15_000 });
    await ctx.storageState({ path: STORAGE_STATE });
    await browser.close();
  }
}
