// E2E: non-postgres backup strategies (mysql, mongo, redis, sqlite). One
// beforeAll deploys all four fixture apps concurrently; each test runs
// seed -> backup -> drop -> restore -> verify against its own app.

import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest, waitForAppStatus } from '../helpers/api.js';
import { findServiceContainer, waitForHealthy } from '../helpers/docker.js';
import { mysqlExec, mongoEval, redisCmd, sqlite3Eval } from '../helpers/dbclients.js';
import {
  createBackupConfig,
  deleteAppConfigsFor,
  localBackupExists,
  localBackupPath,
  readOr,
  triggerAndWait,
} from '../helpers/backups.js';

const FIXTURES = join(import.meta.dirname, '..', 'fixtures');
const MYSQL_PW = 'testrootpass123';
const MONGO_USER = 'root';
const MONGO_PW = 'testrootpass123';
const SQLITE_DB = '/data/test.db';

// ready is a shell probe run in the container until it exits 0.
const APPS = {
  mysql: {
    slug: 'e2e-mysql',
    fixture: 'compose-mysql.yml',
    service: 'db',
    // mysqladmin ping can pass before the root password is set; run a query.
    ready: `mysql -u root -p${MYSQL_PW} -N -B -e 'SELECT 1'`,
  },
  mongo: {
    slug: 'e2e-mongo',
    fixture: 'compose-mongo.yml',
    service: 'db',
    ready: `mongosh --quiet --username ${MONGO_USER} --password ${MONGO_PW} --authenticationDatabase admin --eval "db.runCommand({ ping: 1 }).ok"`,
  },
  redis: { slug: 'e2e-redis', fixture: 'compose-redis.yml', service: 'db', ready: 'redis-cli PING' },
  sqlite: { slug: 'e2e-sqlite', fixture: 'compose-sqlite.yml', service: 'app', ready: `test -f ${SQLITE_DB}` },
};

async function deployAndWait(app) {
  const compose = readFileSync(join(FIXTURES, app.fixture), 'utf-8');
  const res = await apiRequest('POST', '/api/apps/deploy', {
    name: app.slug,
    compose: Buffer.from(compose, 'utf-8').toString('base64'),
  });
  expect(res.status, `deploy ${app.slug}: ${JSON.stringify(res.data)}`).toBe(202);
  app.deployed = true;
  await waitForAppStatus(app.slug, 'running', 240_000);
  app.container = findServiceContainer(app.slug, app.service);
  expect(app.container, `${app.slug} ${app.service} container`).toBeTruthy();
  await waitForHealthy(app.container, app.ready, 180_000);
}

// backupDropRestore takes a local backup of app with strategy, runs drop()
// to destroy the seeded data, restores the backup and returns once the
// restore request is accepted.
async function backupDropRestore(app, strategy, drop, extra = {}) {
  const cfgId = await createBackupConfig(app.slug, { strategy, retention_count: 3, ...extra });
  const run = await triggerAndWait(app.slug, cfgId);
  expect(run.status, `${strategy} run error: ${run.error_msg}`).toBe('success');
  expect(run.size_bytes).toBeGreaterThan(0);
  expect(run.checksum).toBeTruthy();
  expect(localBackupExists(run), `backup file ${localBackupPath(run)}`).toBe(true);

  drop();

  const restoreRes = await apiRequest('POST', `/api/backups/restore/${run.id}`);
  expect(restoreRes.status).toBe(202);
}

test.describe('Backup strategies: MySQL, MongoDB, Redis, SQLite', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeAll(async () => {
    test.setTimeout(420_000);
    const login = await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    expect(login.status).toBe(200);
    await Promise.all(Object.values(APPS).map(deployAndWait));
  });

  test.afterAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
    for (const app of Object.values(APPS).filter((a) => a.deployed)) {
      await deleteAppConfigsFor(app.slug);
      const res = await apiRequest('DELETE', `/api/apps/${app.slug}`);
      expect(res.status, `remove ${app.slug}`).toBe(200);
    }
  });

  test('MySQL: seed -> backup -> drop -> restore -> verify', async () => {
    const c = APPS.mysql.container;
    const select = () => mysqlExec(c, MYSQL_PW, 'testdb', "SELECT CONCAT(id,'|',v) FROM e2e_data WHERE id=1;");
    mysqlExec(c, MYSQL_PW, 'testdb',
      "CREATE TABLE IF NOT EXISTS e2e_data(id INT PRIMARY KEY, v VARCHAR(64)); DELETE FROM e2e_data; INSERT INTO e2e_data VALUES (1, 'mysql-marker-abc');");
    expect(select()).toBe('1|mysql-marker-abc');

    await backupDropRestore(APPS.mysql, 'mysql', () => {
      mysqlExec(c, MYSQL_PW, 'testdb', 'DROP TABLE e2e_data;');
      expect(select).toThrow(/doesn't exist/);
    });

    await expect.poll(() => readOr(select), { timeout: 90_000 }).toBe('1|mysql-marker-abc');
  });

  test('MongoDB: seed -> backup -> drop -> restore -> verify', async () => {
    const c = APPS.mongo.container;
    const find = () => mongoEval(c, MONGO_USER, MONGO_PW, 'JSON.stringify(db.getSiblingDB("testdb").e2e.findOne({_id:1}))');
    mongoEval(c, MONGO_USER, MONGO_PW,
      'db.getSiblingDB("testdb").e2e.deleteMany({}); db.getSiblingDB("testdb").e2e.insertOne({_id:1, v:"mongo-marker-abc"});');
    expect(find()).toContain('mongo-marker-abc');

    await backupDropRestore(APPS.mongo, 'mongo', () => {
      mongoEval(c, MONGO_USER, MONGO_PW, 'db.getSiblingDB("testdb").e2e.drop();');
      expect(mongoEval(c, MONGO_USER, MONGO_PW, 'db.getSiblingDB("testdb").e2e.countDocuments({})')).toBe('0');
    });

    await expect.poll(() => readOr(find), { timeout: 90_000 }).toContain('mongo-marker-abc');
  });

  test('Redis: seed -> backup -> delete -> restore -> verify', async () => {
    const c = APPS.redis.container;
    redisCmd(c, 'SET', 'e2e:marker', 'redis-marker-abc');
    expect(redisCmd(c, 'GET', 'e2e:marker')).toBe('redis-marker-abc');

    await backupDropRestore(APPS.redis, 'redis', () => {
      redisCmd(c, 'DEL', 'e2e:marker');
      expect(redisCmd(c, 'GET', 'e2e:marker')).toBe('');
    });

    // The redis restore stops the container, copies the RDB in and starts it.
    await expect.poll(() => readOr(() => redisCmd(c, 'GET', 'e2e:marker')), { timeout: 60_000 }).toBe('redis-marker-abc');
  });

  test('SQLite: seed -> backup -> delete -> restore -> verify', async () => {
    const c = APPS.sqlite.container;
    const select = () => sqlite3Eval(c, SQLITE_DB, "SELECT id||'|'||v FROM e2e_items WHERE id=1;");
    sqlite3Eval(c, SQLITE_DB, "DELETE FROM e2e_items; INSERT INTO e2e_items(id, v) VALUES (1, 'sqlite-marker-abc');");
    expect(select()).toBe('1|sqlite-marker-abc');

    // The strategy runs `sqlite3 <path> .backup` per path, so pass the DB
    // file, not the volume mount point.
    await backupDropRestore(APPS.sqlite, 'sqlite', () => {
      sqlite3Eval(c, SQLITE_DB, 'DELETE FROM e2e_items;');
      expect(sqlite3Eval(c, SQLITE_DB, 'SELECT COUNT(*) FROM e2e_items;')).toBe('0');
    }, { paths: JSON.stringify([SQLITE_DB]) });

    await expect.poll(() => readOr(select), { timeout: 60_000 }).toBe('1|sqlite-marker-abc');
  });
});
