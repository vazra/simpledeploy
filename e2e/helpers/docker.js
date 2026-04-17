import { execFileSync } from 'child_process';

export function dockerCLI(args, opts = {}) {
  return execFileSync('docker', args, {
    encoding: 'utf-8',
    stdio: ['ignore', 'pipe', 'pipe'],
    ...opts,
  });
}

export function dockerExec(containerName, shellCmd) {
  return dockerCLI(['exec', containerName, 'sh', '-c', shellCmd]);
}

export function dockerExecWithInput(containerName, shellCmd, input) {
  return execFileSync('docker', ['exec', '-i', containerName, 'sh', '-c', shellCmd], {
    encoding: 'utf-8',
    input,
    stdio: ['pipe', 'pipe', 'pipe'],
  });
}

export function dockerInspect(nameOrId) {
  const out = dockerCLI(['inspect', nameOrId]);
  return JSON.parse(out)[0];
}

export function dockerPsNames(filters = []) {
  const args = ['ps', '-a', '--format', '{{.Names}}'];
  for (const f of filters) args.push('--filter', f);
  const out = dockerCLI(args).trim();
  return out ? out.split('\n') : [];
}

export function containerRunning(name) {
  try {
    return dockerInspect(name).State.Running === true;
  } catch {
    return false;
  }
}

export function containerImage(name) {
  return dockerInspect(name).Config.Image;
}

export function containerLabels(name) {
  return dockerInspect(name).Config.Labels || {};
}

export function listAppContainers(appSlug) {
  return dockerPsNames([`label=com.docker.compose.project=simpledeploy-${appSlug}`]);
}

export function findServiceContainer(appSlug, serviceName) {
  const names = dockerPsNames([
    `label=com.docker.compose.project=simpledeploy-${appSlug}`,
    `label=com.docker.compose.service=${serviceName}`,
  ]);
  return names[0] || null;
}

export function countServiceReplicas(appSlug, serviceName) {
  return dockerPsNames([
    `label=com.docker.compose.project=simpledeploy-${appSlug}`,
    `label=com.docker.compose.service=${serviceName}`,
    'status=running',
  ]).length;
}

import { appendFileSync } from 'fs';

export function psql(containerName, user, db, sql) {
  try {
    const out = execFileSync(
      'docker',
      ['exec', containerName, 'psql', '-U', user, '-d', db, '-t', '-A', '-v', 'ON_ERROR_STOP=1', '-c', sql],
      { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] },
    );
    try {
      appendFileSync('/tmp/e2e-psql-trace.log', `[OK ${new Date().toISOString()}] container=${containerName}\nSQL: ${sql}\nOUT: ${JSON.stringify(out)}\n---\n`);
    } catch {}
    return out.trim();
  } catch (e) {
    const stderr = e.stderr ? e.stderr.toString() : '';
    const stdout = e.stdout ? e.stdout.toString() : '';
    try {
      appendFileSync('/tmp/e2e-psql-trace.log', `[ERR ${new Date().toISOString()}] container=${containerName} exitCode=${e.status}\nSQL: ${sql}\nSTDOUT: ${stdout}\nSTDERR: ${stderr}\n---\n`);
    } catch {}
    throw new Error(
      `psql failed container=${containerName} exitCode=${e.status} signal=${e.signal}\nSQL: ${sql}\nSTDOUT: ${stdout}\nSTDERR: ${stderr}`,
    );
  }
}

export async function waitForContainerState(name, running, timeoutMs = 30_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (containerRunning(name) === running) return true;
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`container ${name} did not reach running=${running} within ${timeoutMs}ms`);
}

export async function waitForHealthy(containerName, healthCheckCmd, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  let lastErr;
  while (Date.now() < deadline) {
    try {
      dockerExec(containerName, healthCheckCmd);
      return true;
    } catch (e) {
      lastErr = e;
    }
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`container ${containerName} not healthy after ${timeoutMs}ms: ${lastErr?.message || ''}`);
}
