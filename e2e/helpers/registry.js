// Helpers for running a local Docker registry (registry:2) with htpasswd auth
// for testing private-registry deployments.
//
// Assumes Docker daemon treats 127.0.0.0/8 / localhost as an insecure registry
// (default on Docker Desktop). If the daemon is configured otherwise, the
// push/login calls will fail.

import { execFileSync } from 'child_process';
import { mkdtempSync, writeFileSync, rmSync, existsSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import net from 'net';

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

function docker(args, opts = {}) {
  return execFileSync('docker', args, {
    encoding: 'utf-8',
    stdio: ['ignore', 'pipe', 'pipe'],
    ...opts,
  });
}

function dockerStdin(args, input) {
  return execFileSync('docker', args, {
    encoding: 'utf-8',
    input,
    stdio: ['pipe', 'pipe', 'pipe'],
  });
}

// startRegistry spins up a registry:2 container with htpasswd-based basic auth
// bound to a random localhost port. Returns { host, user, pass, container, stop() }.
export async function startRegistry(opts = {}) {
  const user = opts.user || 'testuser';
  const pass = opts.pass || 'testpass123';
  const port = await getAvailablePort();
  const container = `e2e-registry-${port}`;

  const tmp = mkdtempSync(join(tmpdir(), 'sd-e2e-registry-'));
  const authDir = join(tmp, 'auth');
  execFileSync('mkdir', ['-p', authDir]);

  // Generate htpasswd entry via httpd image (avoids requiring htpasswd on host).
  // httpd's htpasswd -Bbn prints to stdout.
  const htpasswdLine = docker([
    'run', '--rm', '--entrypoint', 'htpasswd', 'httpd:2',
    '-Bbn', user, pass,
  ]).trim();
  writeFileSync(join(authDir, 'htpasswd'), htpasswdLine + '\n');

  // Run registry:2 with htpasswd auth.
  docker([
    'run', '-d',
    '--name', container,
    '-p', `127.0.0.1:${port}:5000`,
    '-v', `${authDir}:/auth`,
    '-e', 'REGISTRY_AUTH=htpasswd',
    '-e', 'REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm',
    '-e', 'REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd',
    'registry:2',
  ]);

  const host = `localhost:${port}`;

  // Wait for registry to be reachable.
  const deadline = Date.now() + 30_000;
  let lastErr;
  while (Date.now() < deadline) {
    try {
      // Unauthenticated call should return 401 (auth configured and listening).
      const out = execFileSync('curl', [
        '-sS', '-o', '/dev/null', '-w', '%{http_code}',
        '--max-time', '2',
        `http://${host}/v2/`,
      ], { encoding: 'utf-8' }).trim();
      if (out === '401' || out === '200') break;
    } catch (e) { lastErr = e; }
    await new Promise((r) => setTimeout(r, 400));
  }

  function stop() {
    try { docker(['rm', '-f', container]); } catch {}
    try { rmSync(tmp, { recursive: true, force: true }); } catch {}
  }

  return { host, user, pass, container, tmp, stop };
}

// pushImage pulls fromImage from docker hub, tags it as `${reg.host}/${toName}`,
// logs in, pushes, then logs out. Returns the full remote image ref.
export function pushImage(reg, fromImage, toName) {
  const target = `${reg.host}/${toName}`;
  try { docker(['pull', fromImage]); } catch (e) {
    throw new Error(`pull ${fromImage} failed: ${e.message}`);
  }
  docker(['tag', fromImage, target]);
  try {
    dockerStdin(
      ['login', '-u', reg.user, '--password-stdin', reg.host],
      reg.pass,
    );
  } catch (e) {
    throw new Error(`docker login ${reg.host} failed: ${e.message}\nstderr: ${e.stderr?.toString() || ''}`);
  }
  try {
    docker(['push', target]);
  } catch (e) {
    throw new Error(`docker push ${target} failed: ${e.message}\nstderr: ${e.stderr?.toString() || ''}`);
  }
  try { docker(['logout', reg.host]); } catch {}
  return target;
}

// Remove a local image (best-effort).
export function rmiLocal(ref) {
  try { docker(['rmi', '-f', ref]); } catch {}
}
