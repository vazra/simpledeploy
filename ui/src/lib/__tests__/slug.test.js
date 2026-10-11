import { describe, it, expect } from 'vitest';
import { isValidSlug, appNameError, APP_NAME_RE } from '../slug.js';

describe('isValidSlug', () => {
  it.each(['a', 'foo', 'my-app', 'my_app.v2', 'A1', 'a'.repeat(63)])('accepts %s', (s) => {
    expect(isValidSlug(s)).toBe(true);
  });

  it.each(['', '.', '..', '../x', '-a', '.a', '_a', 'a/b', 'a?b', 'a#b', 'a b', 'a%2F', 'a'.repeat(64), undefined, null, 42])(
    'rejects %s',
    (s) => {
      expect(isValidSlug(s)).toBe(false);
    },
  );
});

describe('appNameError', () => {
  it.each(['a', 'myapp', 'my-app', 'my_app', '1app', 'app-2', 'a'.repeat(63)])('accepts %s', (s) => {
    expect(appNameError(s)).toBe('');
    expect(APP_NAME_RE.test(s)).toBe(true);
  });

  it('requires a name', () => {
    expect(appNameError('')).toBe('App name is required');
    expect(appNameError('   ')).toBe('App name is required');
    expect(appNameError(undefined)).toBe('App name is required');
  });

  // docker compose rejects these as project names, so the deploy would fail.
  it('rejects uppercase and suggests the lowercase form', () => {
    expect(appNameError('MyApp')).toBe('Use lowercase letters only, for example "myapp"');
  });

  it.each(['my.app', 'my-app_v1.0', 'my app', 'app!'])('rejects %s with the allowed characters', (s) => {
    expect(appNameError(s)).toMatch(/lowercase letters, numbers, hyphens/);
  });

  it.each(['-app', '_app', '.app'])('rejects leading symbol in %s', (s) => {
    expect(appNameError(s)).toBe('Start the name with a letter or number');
  });

  it('rejects names over 63 characters', () => {
    expect(appNameError('a'.repeat(64))).toBe('Keep the name to 63 characters or fewer');
  });
});
