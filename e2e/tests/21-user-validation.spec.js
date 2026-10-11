import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';
import { deleteUserByName, findUser } from '../helpers/users.js';

// Handler rules with no Go coverage in internal/api/users_test.go yet.
test.describe('User Validation', () => {
  test.beforeAll(async () => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
  });

  test.describe('User Creation Validation', () => {
    test('rejects duplicate username', async () => {
      try {
        const res1 = await apiRequest('POST', '/api/users', { username: 'duptest', password: 'TestPass123!', role: 'viewer' });
        expect(res1.status).toBe(201);
        const res2 = await apiRequest('POST', '/api/users', { username: 'duptest', password: 'TestPass123!', role: 'viewer' });
        expect(res2.status).toBe(409);
      } finally {
        await deleteUserByName('duptest');
      }
    });

    test('rejects duplicate email', async () => {
      try {
        const res1 = await apiRequest('POST', '/api/users', {
          username: 'emailtest1', password: 'TestPass123!', role: 'viewer', email: 'dup@test.local',
        });
        expect(res1.status).toBe(201);
        const res2 = await apiRequest('POST', '/api/users', {
          username: 'emailtest2', password: 'TestPass123!', role: 'viewer', email: 'dup@test.local',
        });
        expect(res2.status).toBe(409);
        expect(await findUser('emailtest2')).toBeUndefined();
      } finally {
        await deleteUserByName('emailtest1');
        await deleteUserByName('emailtest2');
      }
    });

    test('rejects short password', async () => {
      const res = await apiRequest('POST', '/api/users', { username: 'shortpw', password: 'short', role: 'viewer' });
      expect(res.status).toBe(400);
      expect(await findUser('shortpw')).toBeUndefined();
    });
  });

  test.describe('User Modification Guards', () => {
    test('admin cannot change own role, and the rejected request changes nothing', async () => {
      const me = await apiRequest('GET', '/api/me');
      expect(me.status).toBe(200);
      expect(typeof me.data.id).toBe('number');
      try {
        const res = await apiRequest('PUT', `/api/users/${me.data.id}`, { role: 'viewer' });
        expect(res.status).toBe(400);
        expect(res.data).toContain('cannot change your own role');

        const after = await apiRequest('GET', '/api/me');
        expect(after.data).toMatchObject({
          role: 'super_admin',
          display_name: TEST_ADMIN.displayName,
          email: TEST_ADMIN.email,
        });
      } finally {
        const restore = await apiRequest('PUT', '/api/me', { display_name: TEST_ADMIN.displayName, email: TEST_ADMIN.email });
        expect(restore.status).toBe(200);
      }
    });

    test('admin cannot delete self', async () => {
      const me = await apiRequest('GET', '/api/me');
      expect(typeof me.data.id).toBe('number');
      const res = await apiRequest('DELETE', `/api/users/${me.data.id}`);
      expect(res.status).toBe(400);
      expect(res.data).toContain('cannot delete yourself');
      expect((await apiRequest('GET', '/api/me')).status).toBe(200);
    });
  });
});
