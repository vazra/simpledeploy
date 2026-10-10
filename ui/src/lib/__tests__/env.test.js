import { describe, it, expect } from 'vitest';
import { MASKED_ENV_PLACEHOLDER, isMaskedEnv, hasMaskedEnv, envDisplayValue, toEnvPayload } from '../env.js';

describe('env helpers', () => {
  it('detects masked entries', () => {
    expect(isMaskedEnv({ key: 'A', value: '', masked: true })).toBe(true);
    expect(isMaskedEnv({ key: 'A', value: '1' })).toBe(false);
    expect(isMaskedEnv(null)).toBe(false);
    expect(hasMaskedEnv([{ key: 'A', value: '1' }, { key: 'B', value: '', masked: true }])).toBe(true);
    expect(hasMaskedEnv([{ key: 'A', value: '1' }])).toBe(false);
    expect(hasMaskedEnv(null)).toBe(false);
  });

  it('displays placeholder for masked, raw value otherwise', () => {
    expect(envDisplayValue({ key: 'A', value: '', masked: true })).toBe(MASKED_ENV_PLACEHOLDER);
    expect(envDisplayValue({ key: 'A', value: 'x' })).toBe('x');
    expect(envDisplayValue({ key: 'A' })).toBe('');
  });

  it('toEnvPayload keeps only key/value', () => {
    expect(toEnvPayload([{ key: 'A', value: '1', masked: false, extra: 1 }])).toEqual([{ key: 'A', value: '1' }]);
    expect(toEnvPayload(null)).toEqual([]);
  });
});
