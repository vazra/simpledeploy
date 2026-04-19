import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

const KEY = 'simpledeploy-theme';

describe('theme store', () => {
  let mediaMatches;

  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
    mediaMatches = false;
    window.matchMedia = (query) => ({
      matches: mediaMatches,
      media: query,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
      onchange: null,
    });
  });

  afterEach(() => {
    document.documentElement.classList.remove('light');
  });

  it('defaults preference to "system" when nothing stored', async () => {
    const { themePreference } = await import('../theme.js?default');
    const { get } = await import('svelte/store');
    expect(get(themePreference)).toBe('system');
  });

  it('reads stored preference', async () => {
    localStorage.setItem(KEY, 'light');
    const { themePreference } = await import('../theme.js?read');
    const { get } = await import('svelte/store');
    expect(get(themePreference)).toBe('light');
  });

  it('derives effective=dark for system pref when media prefers dark', async () => {
    mediaMatches = false; // light query doesn't match -> dark
    const { effectiveTheme } = await import('../theme.js?systemDark');
    const { get } = await import('svelte/store');
    expect(get(effectiveTheme)).toBe('dark');
  });

  it('derives effective=light for system pref when media prefers light', async () => {
    mediaMatches = true;
    const { effectiveTheme } = await import('../theme.js?systemLight');
    const { get } = await import('svelte/store');
    expect(get(effectiveTheme)).toBe('light');
  });

  it('applies the "light" class when preference flips to light', async () => {
    const { themePreference } = await import('../theme.js?apply');
    themePreference.set('light');
    expect(document.documentElement.classList.contains('light')).toBe(true);
    themePreference.set('dark');
    expect(document.documentElement.classList.contains('light')).toBe(false);
  });

  it('persists preference updates to localStorage', async () => {
    const { themePreference } = await import('../theme.js?persist');
    themePreference.set('dark');
    expect(localStorage.getItem(KEY)).toBe('dark');
  });
});
