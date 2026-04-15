import { readFileSync, rmSync, existsSync } from 'fs';
import { join } from 'path';

const STATE_FILE = join(import.meta.dirname, '.e2e-state.json');

export default async function globalTeardown() {
  if (!existsSync(STATE_FILE)) return;

  const state = JSON.parse(readFileSync(STATE_FILE, 'utf-8'));

  try {
    process.kill(state.pid, 'SIGTERM');
    await new Promise((r) => setTimeout(r, 2000));
    try { process.kill(state.pid, 'SIGKILL'); } catch {}
  } catch {}

  for (const dir of [state.dataDir, state.appsDir]) {
    try { rmSync(dir, { recursive: true, force: true }); } catch {}
  }
  try { rmSync(state.configPath, { force: true }); } catch {}
  try { rmSync(STATE_FILE, { force: true }); } catch {}

  console.log('[e2e] Cleanup complete');
}
