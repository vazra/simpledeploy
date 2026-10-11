// Helpers for specs that boot their own SimpleDeploy server (TLS modes, rate
// limits, private registries, disaster recovery) instead of the shared one.
import { copyFileSync, mkdirSync, rmSync } from 'fs';
import { join } from 'path';
import { startServer, getBinaryPath } from './server.js';
import { apiRequestAt } from './api.js';
import { TEST_ADMIN } from './auth.js';

const LOG_DIR = join(import.meta.dirname, '..', 'test-results');

// loginAt logs in against an isolated server and returns the session cookie.
export async function loginAt(baseURL, username = TEST_ADMIN.username, password = TEST_ADMIN.password) {
  const res = await apiRequestAt(baseURL, 'POST', '/api/auth/login', { username, password });
  if (res.status !== 200 || !res.setCookie) {
    throw new Error(`login as ${username} failed: ${res.status} ${JSON.stringify(res.data)}`);
  }
  return res.setCookie;
}

function session(server, cookie) {
  return (method, path, body) => apiRequestAt(server.baseURL, method, path, body, cookie);
}

// startIsolatedServer boots a server on fresh ports (opts go to startServer),
// creates the admin via /api/setup (unless opts.setup === false) and logs in.
// api(method, path, body) is pre-bound to the server and session. If setup
// or login fails the server is stopped, keeping its log under opts.logName.
export async function startIsolatedServer(opts = {}) {
  const { setup = true, admin = TEST_ADMIN, logName, ...serverOpts } = opts;
  const server = await startServer(getBinaryPath(), serverOpts);
  try {
    if (setup) {
      const res = await apiRequestAt(server.baseURL, 'POST', '/api/setup', {
        username: admin.username,
        password: admin.password,
        display_name: admin.displayName,
        email: admin.email,
      });
      if (res.status !== 201) throw new Error(`setup failed: ${res.status} ${JSON.stringify(res.data)}`);
    }
    const cookie = await loginAt(server.baseURL, admin.username, admin.password);
    return { server, cookie, api: session(server, cookie) };
  } catch (e) {
    await stopIsolatedServer(server, logName);
    throw e;
  }
}

// stopGracefully sends SIGTERM and resolves once the process exited, so
// shutdown work (sidecar flush, Caddy stop) has finished. SIGKILL after
// timeoutMs. Data dirs are left in place.
export function stopGracefully(server, timeoutMs = 20_000) {
  const { proc } = server;
  if (!isRunning(server)) return Promise.resolve();
  return new Promise((resolve) => {
    const timer = setTimeout(() => proc.kill('SIGKILL'), timeoutMs);
    proc.once('exit', () => { clearTimeout(timer); resolve(); });
    proc.kill('SIGTERM');
  });
}

export function isRunning(server) {
  return server.proc.exitCode === null && server.proc.signalCode === null;
}

// stopIsolatedServer stops the server, keeps its log as
// test-results/<logName>-server.log (when logName is given) and removes its
// temp dirs.
export async function stopIsolatedServer(server, logName) {
  if (!server) return;
  await stopGracefully(server);
  if (logName) {
    try {
      mkdirSync(LOG_DIR, { recursive: true });
      copyFileSync(server.logPath, join(LOG_DIR, `${logName}-server.log`));
    } catch {}
  }
  for (const dir of [server.dataDir, server.appsDir]) {
    rmSync(dir, { recursive: true, force: true });
  }
}

// deployAt posts a compose file to /api/apps/deploy and asserts the 202.
export async function deployAt(api, name, composeYAML) {
  const res = await api('POST', '/api/apps/deploy', {
    name,
    compose: Buffer.from(composeYAML).toString('base64'),
  });
  if (res.status !== 202) throw new Error(`deploy ${name} failed: ${res.status} ${JSON.stringify(res.data)}`);
  return res;
}

// waitForAppStatusAt polls GET /api/apps/<slug> on an isolated server until
// Status is one of `status` (string or array). Returns the app.
export async function waitForAppStatusAt(baseURL, cookie, slug, status, timeoutMs = 180_000) {
  const want = Array.isArray(status) ? status : [status];
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await apiRequestAt(baseURL, 'GET', `/api/apps/${slug}`, null, cookie);
    last = res;
    // store.App has no JSON tags; fields serialize as PascalCase.
    if (res.status === 200 && want.includes(res.data.Status)) return res.data;
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`app ${slug} did not reach ${want.join('|')} within ${timeoutMs}ms; last: ${last && last.status} ${JSON.stringify(last && last.data)}`);
}
