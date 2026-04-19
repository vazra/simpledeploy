import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { toasts } from '../toast.js';

describe('toast store', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // reset store between tests
    for (const t of get(toasts)) toasts.remove(t.id);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('adds a success toast', () => {
    toasts.success('all good');
    const all = get(toasts);
    expect(all).toHaveLength(1);
    expect(all[0].type).toBe('success');
    expect(all[0].message).toBe('all good');
  });

  it('emits distinct ids across add calls', () => {
    toasts.info('a');
    toasts.info('b');
    const all = get(toasts);
    expect(new Set(all.map((t) => t.id)).size).toBe(all.length);
  });

  it('wraps error/warning/info types', () => {
    toasts.error('x');
    toasts.warning('y');
    toasts.info('z');
    const types = get(toasts).map((t) => t.type);
    expect(types).toEqual(['error', 'warning', 'info']);
  });

  it('remove() deletes by id', () => {
    toasts.success('gone');
    const id = get(toasts)[0].id;
    toasts.remove(id);
    expect(get(toasts)).toHaveLength(0);
  });

  it('remove() on an unknown id is a no-op', () => {
    toasts.success('keep');
    toasts.remove(999999);
    expect(get(toasts)).toHaveLength(1);
  });

  it('auto-removes after the default 4s timeout', () => {
    toasts.success('bye');
    expect(get(toasts)).toHaveLength(1);
    vi.advanceTimersByTime(3999);
    expect(get(toasts)).toHaveLength(1);
    vi.advanceTimersByTime(2);
    expect(get(toasts)).toHaveLength(0);
  });
});
