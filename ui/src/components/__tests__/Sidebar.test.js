import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import Sidebar from '../Sidebar.svelte';
import { sidebarExpanded } from '../../lib/stores/sidebar.js';
import { get } from 'svelte/store';

describe('Sidebar', () => {
  beforeEach(() => {
    sidebarExpanded.set(true);
    window.location.hash = '#/';
  });

  it('renders all primary nav items', () => {
    const { getByText } = render(Sidebar);
    for (const label of ['Dashboard', 'Alerts', 'Backups', 'Users', 'Registries', 'Docker', 'System']) {
      expect(getByText(label)).toBeInTheDocument();
    }
  });

  it('marks Dashboard active when hash is /', () => {
    const { container } = render(Sidebar);
    const links = container.querySelectorAll('a');
    const dashboardLink = Array.from(links).find((a) => a.textContent.includes('Dashboard'));
    expect(dashboardLink.className).toMatch(/bg-surface-3\/50/);
  });

  it('toggles sidebar expansion when the toggle clicked', async () => {
    const { getByLabelText } = render(Sidebar);
    expect(get(sidebarExpanded)).toBe(true);
    await fireEvent.click(getByLabelText('Toggle sidebar'));
    expect(get(sidebarExpanded)).toBe(false);
  });

  it('forceExpanded overrides the store', () => {
    sidebarExpanded.set(false);
    const { getByText } = render(Sidebar, { forceExpanded: true });
    expect(getByText('SimpleDeploy')).toBeInTheDocument();
  });

  it('updates active link on hashchange', async () => {
    const { container } = render(Sidebar);
    window.location.hash = '#/alerts';
    window.dispatchEvent(new HashChangeEvent('hashchange'));
    await tick();
    const links = container.querySelectorAll('a');
    const active = Array.from(links).find((a) => a.textContent.includes('Alerts'));
    expect(active.className).toMatch(/bg-surface-3\/50/);
  });
});
