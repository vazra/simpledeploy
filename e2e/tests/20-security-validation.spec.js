import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';

const SECURITY_FIXTURES = join(import.meta.dirname, '..', 'fixtures', 'security');

function readAndEncode(filename) {
  const content = readFileSync(join(SECURITY_FIXTURES, filename), 'utf-8');
  return Buffer.from(content).toString('base64');
}

function encodeCompose(yaml) {
  return Buffer.from(yaml).toString('base64');
}

const VALID_COMPOSE = encodeCompose('services:\n  web:\n    image: nginx:alpine\n');

test.describe('Security Validation', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  // Compose security tests - each fixture should be rejected
  test.describe('Compose Security', () => {
    test('rejects privileged containers', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-priv', compose: readAndEncode('compose-privileged.yml')
      });
      expect(res.status).toBe(400);
      expect(res.data.violations).toBeDefined();
      expect(res.data.violations.some(v => v.includes('privileged'))).toBeTruthy();
    });

    test('rejects host network mode', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-net', compose: readAndEncode('compose-host-network.yml')
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('network_mode'))).toBeTruthy();
    });

    test('rejects host pid mode', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-pid', compose: readAndEncode('compose-host-pid.yml')
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('pid'))).toBeTruthy();
    });

    test('rejects host ipc mode', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-ipc', compose: readAndEncode('compose-host-ipc.yml')
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('ipc'))).toBeTruthy();
    });

    test('rejects dangerous capabilities', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-caps', compose: readAndEncode('compose-dangerous-caps.yml')
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('SYS_ADMIN'))).toBeTruthy();
    });

    test('rejects cap_add ALL', async () => {
      const compose = encodeCompose('services:\n  web:\n    image: nginx:alpine\n    cap_add:\n      - ALL\n');
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-capall', compose
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('ALL'))).toBeTruthy();
    });

    test('rejects cap_add NET_ADMIN', async () => {
      const compose = encodeCompose('services:\n  web:\n    image: nginx:alpine\n    cap_add:\n      - NET_ADMIN\n');
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-netadm', compose
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('NET_ADMIN'))).toBeTruthy();
    });

    test('rejects dangerous volume mounts', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-vol', compose: readAndEncode('compose-dangerous-volumes.yml')
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('docker.sock'))).toBeTruthy();
    });

    test('rejects bind mount of /etc', async () => {
      const compose = encodeCompose('services:\n  web:\n    image: nginx:alpine\n    volumes:\n      - /etc:/host-etc\n');
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-etc', compose
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('/etc'))).toBeTruthy();
    });

    test('rejects bind mount of /proc', async () => {
      const compose = encodeCompose('services:\n  web:\n    image: nginx:alpine\n    volumes:\n      - /proc:/host-proc\n');
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-proc', compose
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('/proc'))).toBeTruthy();
    });

    test('rejects bind mount of /root', async () => {
      const compose = encodeCompose('services:\n  web:\n    image: nginx:alpine\n    volumes:\n      - /root:/host-root\n');
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-root', compose
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.some(v => v.includes('/root'))).toBeTruthy();
    });

    test('reports multiple violations together', async () => {
      const compose = encodeCompose('services:\n  web:\n    image: nginx:alpine\n    privileged: true\n    network_mode: host\n');
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-multi', compose
      });
      expect(res.status).toBe(400);
      expect(res.data.violations.length).toBeGreaterThanOrEqual(2);
    });

    test('accepts valid compose without violations', async () => {
      // Valid compose should not be rejected for security (may deploy or 500 without docker, but not 400)
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'sec-test-valid', compose: VALID_COMPOSE
      });
      expect(res.status).not.toBe(400);
    });
  });

  // App name validation tests
  test.describe('App Name Validation', () => {
    test('rejects empty name', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: '', compose: VALID_COMPOSE
      });
      expect(res.status).toBe(400);
    });

    test('rejects name starting with dash', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: '-invalid', compose: VALID_COMPOSE
      });
      expect(res.status).toBe(400);
      expect(res.data).toContain('invalid app name');
    });

    test('rejects name starting with dot', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: '.invalid', compose: VALID_COMPOSE
      });
      expect(res.status).toBe(400);
    });

    test('rejects name with spaces', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'my app', compose: VALID_COMPOSE
      });
      expect(res.status).toBe(400);
    });

    test('rejects name exceeding 63 chars', async () => {
      const name = 'a' + 'b'.repeat(63); // 64 chars
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name, compose: VALID_COMPOSE
      });
      expect(res.status).toBe(400);
    });

    test('accepts name at 63 char boundary', async () => {
      const name = 'a' + 'b'.repeat(62); // 63 chars
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name, compose: VALID_COMPOSE
      });
      // Should not get 400 for name validation (may get other errors)
      if (res.status === 400) {
        expect(res.data).not.toContain('invalid app name');
      }
    });

    test('accepts name with dots dashes underscores', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'my-app_v1.0', compose: VALID_COMPOSE
      });
      if (res.status === 400) {
        expect(res.data).not.toContain('invalid app name');
      }
    });

    test('rejects missing compose field', async () => {
      const res = await apiRequest('POST', '/api/apps/deploy', {
        name: 'test-app', compose: ''
      });
      expect(res.status).toBe(400);
    });
  });

  // Cleanup any apps that may have been created during valid compose test
  test('cleanup security test apps', async () => {
    for (const name of ['sec-test-valid', 'my-app_v1.0']) {
      await apiRequest('DELETE', `/api/apps/${name}`);
    }
    // Also clean up the 63-char app name
    const name63 = 'a' + 'b'.repeat(62);
    await apiRequest('DELETE', `/api/apps/${name63}`);
  });
});
