import { execFileSync, spawn } from 'child_process';
import { mkdtempSync, writeFileSync, rmSync, existsSync, readFileSync, createWriteStream } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import net from 'net';

const ROOT = join(import.meta.dirname, '..', '..');

function getAvailablePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, () => {
      const port = srv.address().port;
      srv.close(() => resolve(port));
    });
    srv.on('error', reject);
  });
}

export async function buildBinary() {
  console.log('[e2e] Building SimpleDeploy binary...');
  execFileSync('make', ['build'], { cwd: ROOT, stdio: 'inherit' });
  const bin = join(ROOT, 'bin', 'simpledeploy');
  if (!existsSync(bin)) throw new Error('Binary not found after build');
  return bin;
}

// getBinaryPath returns the expected binary path without rebuilding.
// Use this in tests that need an isolated server after global-setup has built the binary.
export function getBinaryPath() {
  return join(ROOT, 'bin', 'simpledeploy');
}

export async function startServer(binPath, overrides = {}) {
  const port = await getAvailablePort();
  const proxyPort = overrides.proxyPort || await getAvailablePort();
  const dataDir = overrides.dataDir || mkdtempSync(join(tmpdir(), 'sd-e2e-data-'));
  const appsDir = overrides.appsDir || mkdtempSync(join(tmpdir(), 'sd-e2e-apps-'));
  const configPath = join(dataDir, 'config.yml');
  const logPath = join(dataDir, 'server.log');
  const tlsMode = overrides.tlsMode || 'off';
  // ratelimit override only affects the auth/API rate limiter (cfg.RateLimit).
  // Per-app proxy rate limits come from compose labels (simpledeploy.ratelimit.*).
  const rl = overrides.ratelimit || {};
  const rlRequests = rl.requests ?? 10000;
  const rlWindow = rl.window ?? '60s';
  const rlBurst = rl.burst ?? 5000;
  const rlBy = rl.by ?? 'ip';

  const config = [
    `data_dir: "${dataDir}"`,
    `apps_dir: "${appsDir}"`,
    `listen_addr: ":${proxyPort}"`,
    `management_port: ${port}`,
    `master_secret: "e2e-test-secret-key-32bytes!!"`,
    `log_buffer_size: 100`,
    `tls:`,
    `  mode: "${tlsMode}"`,
    `ratelimit:`,
    `  requests: ${rlRequests}`,
    `  window: "${rlWindow}"`,
    `  burst: ${rlBurst}`,
    `  by: "${rlBy}"`,
    // Per-IP login rate limit. Production default is 10/min; tests log in
    // many times in rapid succession from 127.0.0.1, so raise it.
    `login_ratelimit:`,
    `  requests: 10000`,
    `  window: "60s"`,
  ].join('\n');

  writeFileSync(configPath, config);

  console.log(`[e2e] Starting server on port ${port}...`);
  console.log(`[e2e] Data dir: ${dataDir}`);
  console.log(`[e2e] Apps dir: ${appsDir}`);

  const logStream = createWriteStream(logPath);
  // Optional GHCR mirror for template/fixture images. Avoids Docker Hub
  // pull rate limits on shared CI tokens and local dev. Off unless
  // E2E_USE_MIRROR=1 is set in the environment.
  const mirrorEnv = {};
  if (process.env.E2E_USE_MIRROR === '1') {
    mirrorEnv.SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX =
      process.env.SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX || 'ghcr.io/vazra/simpledeploy-mirror/';
  }
  // Turn the knobs down for e2e: production tickers are tuned for low load;
  // the tests need to observe state transitions quickly to stay under the
  // 360s test-level budget.
  const fastEnv = {
    SIMPLEDEPLOY_METRICS_FLUSH_INTERVAL: '1s',
    SIMPLEDEPLOY_REQUEST_METRICS_FLUSH_INTERVAL: '1s',
    SIMPLEDEPLOY_ALERT_EVAL_INTERVAL: '2s',
    SIMPLEDEPLOY_STATUS_REFRESH_INTERVAL: '5s',
    SIMPLEDEPLOY_STABILIZE_TIMEOUT: '10s',
  };
  const proc = spawn(binPath, ['serve', '--config', configPath], {
    cwd: ROOT,
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS: '1', ...fastEnv, ...mirrorEnv },
  });

  proc.stdout.pipe(logStream);
  proc.stderr.pipe(logStream);

  proc.on('error', (err) => {
    console.error('[e2e] Server process error:', err);
  });

  const baseURL = `http://localhost:${port}`;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseURL}/api/health`);
      if (res.ok) {
        console.log(`[e2e] Server healthy at ${baseURL} (proxy :${proxyPort})`);
        return { proc, port, proxyPort, dataDir, appsDir, configPath, logPath, baseURL, proxyURL: `http://localhost:${proxyPort}` };
      }
    } catch {}
    await new Promise((r) => setTimeout(r, 300));
  }

  proc.kill('SIGKILL');
  const logs = existsSync(logPath) ? readFileSync(logPath, 'utf-8') : '(no logs)';
  throw new Error(`Server failed to start within 30s.\nLogs:\n${logs}`);
}

export function stopServer(server) {
  if (!server) return;
  console.log('[e2e] Stopping server...');
  try { server.proc.kill('SIGTERM'); } catch {}
  for (const dir of [server.dataDir, server.appsDir]) {
    try { rmSync(dir, { recursive: true, force: true }); } catch {}
  }
  try { rmSync(server.configPath, { force: true }); } catch {}
}
