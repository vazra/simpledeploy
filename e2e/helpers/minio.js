import { execFileSync, spawnSync } from 'child_process';
import net from 'net';

// MinIO fixture helper. Starts a MinIO server in a docker container on a random
// host port, waits for health, creates a bucket, and exposes helpers to list /
// remove objects via the `minio/mc` client image (keeps deps light: no S3 SDK).

const MINIO_IMAGE = 'minio/minio:latest';
const MC_IMAGE = 'minio/mc:latest';

export const MINIO_ROOT_USER = 'minioadmin';
export const MINIO_ROOT_PASSWORD = 'minioadmin123';

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

export function dockerAvailable() {
  try {
    execFileSync('docker', ['version', '--format', '{{.Server.Version}}'], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    return true;
  } catch {
    return false;
  }
}

async function waitForHealth(port, timeoutMs = 45_000) {
  const deadline = Date.now() + timeoutMs;
  let lastErr;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://localhost:${port}/minio/health/live`);
      if (res.ok) return true;
      lastErr = new Error(`health status ${res.status}`);
    } catch (e) {
      lastErr = e;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`MinIO not healthy after ${timeoutMs}ms: ${lastErr?.message || ''}`);
}

// startMinIO starts a MinIO container and creates the given bucket.
// Returns {host, port, endpoint, accessKey, secretKey, containerName, bucket,
// mc(args), listObjects(), stop()}.
export async function startMinIO({ bucket = 'e2e-backups' } = {}) {
  if (!dockerAvailable()) {
    throw new Error('docker not available');
  }

  const port = await getAvailablePort();
  const consolePort = await getAvailablePort();
  const containerName = `sd-e2e-minio-${Date.now()}-${Math.floor(Math.random() * 10000)}`;

  execFileSync(
    'docker',
    [
      'run', '-d', '--rm',
      '--name', containerName,
      '-p', `${port}:9000`,
      '-p', `${consolePort}:9001`,
      '-e', `MINIO_ROOT_USER=${MINIO_ROOT_USER}`,
      '-e', `MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}`,
      MINIO_IMAGE,
      'server', '/data', '--console-address', ':9001',
    ],
    { stdio: ['ignore', 'pipe', 'pipe'] },
  );

  try {
    await waitForHealth(port, 60_000);
  } catch (e) {
    try { execFileSync('docker', ['rm', '-f', containerName], { stdio: 'ignore' }); } catch {}
    throw e;
  }

  const host = 'localhost';
  const endpoint = `http://${host}:${port}`;
  const accessKey = MINIO_ROOT_USER;
  const secretKey = MINIO_ROOT_PASSWORD;

  // mc runs the minio client in a short-lived container, configured via env
  // MC_HOST_local. Use host networking via host.docker.internal on mac/windows,
  // or --add-host for linux. We pass the endpoint as http://host.docker.internal:PORT
  // because mc runs inside docker.
  function mc(args, opts = {}) {
    const mcHost = `http://${accessKey}:${secretKey}@host.docker.internal:${port}`;
    try {
      return execFileSync(
        'docker',
        [
          'run', '--rm',
          '--add-host', 'host.docker.internal:host-gateway',
          '-e', `MC_HOST_local=${mcHost}`,
          MC_IMAGE,
          ...args,
        ],
        { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'], ...opts },
      );
    } catch (e) {
      const stderr = e.stderr ? e.stderr.toString() : '';
      const stdout = e.stdout ? e.stdout.toString() : '';
      throw new Error(`mc ${args.join(' ')} failed: ${stderr || stdout || e.message}`);
    }
  }

  // Create the bucket via mc.
  mc(['mb', '--ignore-existing', `local/${bucket}`]);

  function listObjects(prefix = '') {
    const target = prefix ? `local/${bucket}/${prefix}` : `local/${bucket}`;
    // mc ls --json emits one JSON document per line.
    let out;
    try {
      out = mc(['ls', '--json', target]);
    } catch (e) {
      // Empty listing can return non-zero on some mc versions; treat as empty.
      if (/no such|not found/i.test(String(e.message))) return [];
      // Fallback: try plain ls
      try {
        const plain = mc(['ls', target]);
        return plain
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean)
          .map((line) => {
            const parts = line.split(/\s+/);
            return { key: parts[parts.length - 1], size: 0 };
          });
      } catch {
        return [];
      }
    }
    const entries = [];
    for (const line of out.split('\n')) {
      const s = line.trim();
      if (!s) continue;
      try {
        const obj = JSON.parse(s);
        if (obj.key || obj.name) {
          entries.push({ key: obj.key || obj.name, size: obj.size || 0 });
        }
      } catch {
        // ignore non-json lines
      }
    }
    return entries;
  }

  function stop() {
    try {
      execFileSync('docker', ['rm', '-f', containerName], {
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch {}
  }

  return {
    host,
    port,
    endpoint,
    accessKey,
    secretKey,
    bucket,
    containerName,
    mc,
    listObjects,
    stop,
  };
}
