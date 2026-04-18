// DB client helpers for non-postgres backup e2e tests.
// Each helper spawns `docker exec` with arguments passed as argv (not shell),
// matching the pattern used by psql() in helpers/docker.js to avoid quoting hazards.

import { execFileSync } from 'child_process';
import { appendFileSync } from 'fs';

function trace(tag, container, payload, outOrErr) {
  try {
    appendFileSync(
      '/tmp/e2e-dbclients-trace.log',
      `[${tag} ${new Date().toISOString()}] container=${container}\nCMD: ${payload}\nOUT: ${JSON.stringify(outOrErr)}\n---\n`,
    );
  } catch {}
}

function runArgv(container, argv, stdinBuf) {
  const opts = { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] };
  if (stdinBuf !== undefined) {
    opts.stdio = ['pipe', 'pipe', 'pipe'];
    opts.input = stdinBuf;
  }
  try {
    const out = execFileSync('docker', argv, opts);
    trace('OK', container, argv.join(' '), out);
    return out.trim();
  } catch (e) {
    const stderr = e.stderr ? e.stderr.toString() : '';
    const stdout = e.stdout ? e.stdout.toString() : '';
    trace('ERR', container, argv.join(' '), { status: e.status, stdout, stderr });
    throw new Error(
      `docker exec failed container=${container} exitCode=${e.status}\nARGV: ${argv.join(' ')}\nSTDOUT: ${stdout}\nSTDERR: ${stderr}`,
    );
  }
}

// mysqlExec runs SQL against a MySQL container as root.
// Use `db` = '' or null for statements that don't need a database.
// Retries on ERROR 2002 (socket not ready) which can occur briefly during
// the initial container init when mysqld restarts.
export function mysqlExec(container, password, db, sql) {
  const argv = ['exec', container, 'mysql', '-u', 'root', `-p${password}`, '-N', '-B'];
  if (db) argv.push(db);
  argv.push('-e', sql);
  const maxAttempts = 5;
  for (let i = 0; i < maxAttempts; i++) {
    try {
      return runArgv(container, argv);
    } catch (e) {
      const msg = String(e.message || '');
      const transient = msg.includes('ERROR 2002') || msg.includes('MySQL server has gone away');
      if (!transient || i === maxAttempts - 1) throw e;
      execFileSync('sleep', ['2']);
    }
  }
}

// mongoEval runs a JS snippet via mongosh using root auth.
// Returns stdout trimmed. For queries returning documents use EJSON.stringify(...) inside `js`.
export function mongoEval(container, user, password, js) {
  const argv = [
    'exec', container, 'mongosh',
    '--host', 'localhost',
    '--username', user,
    '--password', password,
    '--authenticationDatabase', 'admin',
    '--quiet',
    '--eval', js,
  ];
  return runArgv(container, argv);
}

// redisCmd runs a single redis-cli command.
// Pass extra args positionally: redisCmd(name, 'SET', 'foo', 'bar').
export function redisCmd(container, ...args) {
  const argv = ['exec', container, 'redis-cli', ...args];
  return runArgv(container, argv);
}

// sqlite3Eval runs a SQL statement against a sqlite DB file inside the container.
export function sqlite3Eval(container, dbPath, sql) {
  const argv = ['exec', container, 'sqlite3', dbPath, sql];
  return runArgv(container, argv);
}
