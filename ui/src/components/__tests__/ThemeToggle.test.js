import { describe, it, expect, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';
import ThemeToggle from '../ThemeToggle.svelte';
import { themePreference } from '../../lib/stores/theme.js';

describe('ThemeToggle', () => {
  beforeEach(() => {
    themePreference.set('system');
  });

  it('cycles system -> light -> dark -> system', async () => {
    const { getByRole } = render(ThemeToggle);
    const btn = getByRole('button');
    await fireEvent.click(btn);
    expect(get(themePreference)).toBe('light');
    await fireEvent.click(btn);
    expect(get(themePreference)).toBe('dark');
    await fireEvent.click(btn);
    expect(get(themePreference)).toBe('system');
  });

  it('has an aria-label for accessibility', () => {
    const { getByLabelText } = render(ThemeToggle);
    expect(getByLabelText('Toggle theme')).toBeInTheDocument();
  });

  it('shows a system-indicator dot in system mode', () => {
    themePreference.set('system');
    const { container } = render(ThemeToggle);
    expect(container.querySelector('.bg-accent.rounded-full')).not.toBeNull();
  });

  it('hides the system-indicator dot when a specific mode is chosen', () => {
    themePreference.set('light');
    const { container } = render(ThemeToggle);
    expect(container.querySelector('.bg-accent.rounded-full')).toBeNull();
  });
});
