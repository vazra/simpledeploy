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
    test('locks out after 10 failed attempts', async () => {
      // Create temp user for lockout testing
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
      const createRes = await apiRequest('POST', '/api/users', {
        username: 'lockouttest', password: 'LockoutPass123!', role: 'viewer',
      });

      // Send 10 failed login attempts
      for (let i = 0; i < 10; i++) {
        await apiRequest('POST', '/api/auth/login', {
          username: 'lockouttest', password: 'wrong-password',
        });
      }

      // 11th attempt should be locked out
      const res = await apiRequest('POST', '/api/auth/login', {
        username: 'lockouttest', password: 'wrong-password',
      });
      expect(res.status).toBe(429);

      // Cleanup: re-login as admin and delete temp user
      await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password);
      if (createRes.ok && createRes.data?.id) {
        await apiRequest('DELETE', `/api/users/${createRes.data.id}`);
      }
    });
  });
});
