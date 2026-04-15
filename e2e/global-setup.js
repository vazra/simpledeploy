import { buildBinary, startServer } from './helpers/server.js';
import { writeFileSync } from 'fs';
import { join } from 'path';

const STATE_FILE = join(import.meta.dirname, '.e2e-state.json');

export default async function globalSetup() {
  const binPath = await buildBinary();
  const server = await startServer(binPath);

  const state = {
    pid: server.proc.pid,
    port: server.port,
    dataDir: server.dataDir,
    appsDir: server.appsDir,
    configPath: server.configPath,
    logPath: server.logPath,
    baseURL: server.baseURL,
  };
  writeFileSync(STATE_FILE, JSON.stringify(state));

  process.env.SIMPLEDEPLOY_PORT = String(server.port);
}
