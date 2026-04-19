import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

const KEY = 'simpledeploy-sidebar';

describe('sidebar store', () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('reads initial state from localStorage when set', async () => {
    localStorage.setItem(KEY, 'false');
    const { sidebarExpanded } = await import('../sidebar.js?forceSet=false');
    const { get } = await import('svelte/store');
    expect(get(sidebarExpanded)).toBe(false);
  });

  it('defaults to true on wide viewports (>=1024)', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1200 });
    const { sidebarExpanded } = await import('../sidebar.js?wide');
    const { get } = await import('svelte/store');
    expect(get(sidebarExpanded)).toBe(true);
  });

  it('defaults to false on narrow viewports (<1024)', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 800 });
    const { sidebarExpanded } = await import('../sidebar.js?narrow');
    const { get } = await import('svelte/store');
    expect(get(sidebarExpanded)).toBe(false);
  });

  it('writes to localStorage on update', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1200 });
    const { sidebarExpanded } = await import('../sidebar.js?write');
    sidebarExpanded.set(false);
    expect(localStorage.getItem(KEY)).toBe('false');
    sidebarExpanded.set(true);
    expect(localStorage.getItem(KEY)).toBe('true');
  });
});
