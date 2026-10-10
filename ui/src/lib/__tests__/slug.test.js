import { describe, it, expect } from 'vitest';
import { isValidSlug } from '../slug.js';

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
