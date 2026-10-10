import { describe, it, expect } from 'vitest';
import { errorLines, formatApiError, violationsOf } from '../apiErrors.js';

const refused = {
  error: 'compose file contains disallowed directives',
  violations: ['service "web": privileged not allowed', 'service "web": pid "host" not allowed'],
};
const expected = 'compose file contains disallowed directives:\n- service "web": privileged not allowed\n- service "web": pid "host" not allowed';

describe('apiErrors', () => {
  it('formatApiError appends violations as a bullet list', () => {
    expect(formatApiError(refused, 'fallback')).toBe(expected);
  });

  it('formatApiError trims trailing punctuation before the list', () => {
    expect(formatApiError({ error: 'refused.', violations: ['a'] }, 'x')).toBe('refused:\n- a');
  });

  it('formatApiError uses error alone when there are no violations', () => {
    expect(formatApiError({ error: 'nope' }, 'x')).toBe('nope');
    expect(formatApiError({ error: 'nope', violations: [] }, 'x')).toBe('nope');
  });

  it('formatApiError falls back when there is no error field', () => {
    expect(formatApiError(null, 'plain text')).toBe('plain text');
    expect(formatApiError({ violations: ['a'] }, '')).toBe('Request refused:\n- a');
  });

  it('violationsOf ignores non-arrays and non-strings', () => {
    expect(violationsOf({ violations: 'nope' })).toEqual([]);
    expect(violationsOf({ violations: ['a', 3, '', null, 'b'] })).toEqual(['a', 'b']);
    expect(violationsOf(null)).toEqual([]);
  });

  it('errorLines splits a refusal into one line per reason', () => {
    expect(errorLines(expected)).toEqual([
      'compose file contains disallowed directives:',
      '- service "web": privileged not allowed',
      '- service "web": pid "host" not allowed',
    ]);
    expect(errorLines('one line\n')).toEqual(['one line']);
    expect(errorLines(null)).toEqual([]);
  });
});
