import { test, expect } from '@playwright/test';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { TEST_ADMIN } from '../helpers/auth.js';

test.describe('User Validation', () => {
  test.beforeAll(async () => {
    await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
  });

  test.describe('User Creation Validation', () => {
    test('rejects duplicate username', async () => {
      // First create a user
      const res1 = await apiRequest('POST', '/api/users', {
        username: 'duptest', password: 'TestPass123!', role: 'viewer',
      });
      expect(res1.status).toBe(201);

      // Try creating with same username
      const res2 = await apiRequest('POST', '/api/users', {
        username: 'duptest', password: 'TestPass123!', role: 'viewer',
      });
      expect(res2.status).toBe(409);

      // Cleanup
      if (res1.ok && res1.data?.id) {
        await apiRequest('DELETE', `/api/users/${res1.data.id}`);
      }
    });

    test('rejects duplicate email', async () => {
      const res1 = await apiRequest('POST', '/api/users', {
        username: 'emailtest1', password: 'TestPass123!', role: 'viewer',
        email: 'dup@test.local',
      });
      expect(res1.status).toBe(201);

      const res2 = await apiRequest('POST', '/api/users', {
        username: 'emailtest2', password: 'TestPass123!', role: 'viewer',
        email: 'dup@test.local',
      });
      expect(res2.status).toBe(409);

      // Cleanup
      if (res1.ok && res1.data?.id) {
        await apiRequest('DELETE', `/api/users/${res1.data.id}`);
      }
    });

    test('rejects short password', async () => {
      const res = await apiRequest('POST', '/api/users', {
        username: 'shortpw', password: 'short', role: 'viewer',
      });
      expect(res.status).toBe(400);
    });

    test('creates valid user successfully', async () => {
      const res = await apiRequest('POST', '/api/users', {
        username: 'validuser', password: 'ValidPass123!', role: 'viewer',
        display_name: 'Valid User', email: 'valid@test.local',
      });
      expect(res.status).toBe(201);
      expect(res.data.username).toBe('validuser');

      // Cleanup
      if (res.ok && res.data?.id) {
        await apiRequest('DELETE', `/api/users/${res.data.id}`);
      }
    });
  });

  test.describe('User Modification Guards', () => {
    test('admin cannot change own role', async () => {
      // Get current user ID
      const me = await apiRequest('GET', '/api/me');
      expect(me.ok).toBeTruthy();

      const res = await apiRequest('PUT', `/api/users/${me.data.id}`, {
        role: 'viewer',
      });
      expect(res.status).toBe(400);
    });

    test('admin cannot delete self', async () => {
      const me = await apiRequest('GET', '/api/me');
      expect(me.ok).toBeTruthy();

      const res = await apiRequest('DELETE', `/api/users/${me.data.id}`);
      expect(res.status).toBe(400);
    });
  });

  test.describe('Login Lockout', () => {
    test('wrong password returns 401', async () => {
      // Verify failed login returns 401 (not testing full lockout to avoid
      // locking the IP and breaking subsequent tests)
      const res = await apiRequest('POST', '/api/auth/login', {
        username: 'nonexistent', password: 'wrong-password',
      });
      expect(res.status).toBe(401);
    });
  });
});
