import { readFileSync, rmSync, existsSync, mkdirSync, copyFileSync } from 'fs';
import { execFileSync } from 'child_process';
import { tmpdir } from 'os';
import { join } from 'path';

const STATE_FILE = join(import.meta.dirname, '.e2e-state.json');
const LOG_DIR = join(import.meta.dirname, 'test-results');

// Compose projects deployed by e2e servers (shared and isolated) live under
// temp dirs named sd-e2e-*. Only those are removed, never other stacks on
// the machine. Specs clean up after themselves; this catches what a failed
// spec leaked.
function removeLeakedStacks() {
  let out;
  try {
    out = execFileSync('docker', [
      'ps', '-a', '--filter', 'label=com.docker.compose.project',
      '--format', '{{.Label "com.docker.compose.project"}}\t{{.Label "com.docker.compose.project.working_dir"}}',
    ], { encoding: 'utf-8' });
  } catch {
    return;
  }
  const prefix = join(tmpdir(), 'sd-e2e-');
  const projects = new Set();
  for (const line of out.split('\n')) {
    const [project, dir] = line.split('\t');
    if (project && dir && (dir.startsWith(prefix) || dir.startsWith(`/private${prefix}`))) projects.add(project);
  }
  for (const p of projects) {
    try {
      execFileSync('docker', ['compose', '-p', p, 'down', '-v', '--remove-orphans'], { stdio: 'ignore', timeout: 60_000 });
      console.log(`[e2e] Removed leaked stack ${p}`);
    } catch {}
  }
}

export default async function globalTeardown() {
  if (!existsSync(STATE_FILE)) return;

  const state = JSON.parse(readFileSync(STATE_FILE, 'utf-8'));

  try {
    process.kill(state.pid, 'SIGTERM');
    await new Promise((r) => setTimeout(r, 2000));
    try { process.kill(state.pid, 'SIGKILL'); } catch {}
  } catch {}

  // Keep the server log next to the failure traces (CI uploads test-results/).
  try {
    mkdirSync(LOG_DIR, { recursive: true });
    copyFileSync(state.logPath, join(LOG_DIR, 'server.log'));
  } catch {}

  removeLeakedStacks();

  if (!process.env.SIMPLEDEPLOY_KEEP) {
    for (const dir of [state.dataDir, state.appsDir]) {
      try { rmSync(dir, { recursive: true, force: true }); } catch {}
    }
  } else {
    console.log('[e2e] Keeping data dir:', state.dataDir, 'apps:', state.appsDir);
  }
  try { rmSync(state.configPath, { force: true }); } catch {}
  try { rmSync(STATE_FILE, { force: true }); } catch {}

  console.log('[e2e] Cleanup complete');
}
