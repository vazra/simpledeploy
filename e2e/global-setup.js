import { buildBinary, startServer } from './helpers/server.js';
import { writeFileSync } from 'fs';
import { join } from 'path';

const STATE_FILE = join(import.meta.dirname, '.e2e-state.json');

export default async function globalSetup() {
  const binPath = await buildBinary();
  const server = await startServer(binPath);

  // Safety net: kill server if this process exits unexpectedly
  // (e.g., global-setup crashes after spawn but before teardown runs)
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
}
