import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      getProfile: vi.fn(async () => ({
        data: { username: 'admin', display_name: 'Admin', role: 'super_admin', app_access: [] },
        error: null,
        status: 200,
      })),
    }),
  };
});

import Sidebar from '../Sidebar.svelte';
import { sidebarExpanded } from '../../lib/stores/sidebar.js';
import { get } from 'svelte/store';

describe('Sidebar', () => {
  beforeEach(() => {
    sidebarExpanded.set(true);
    window.location.hash = '#/';
  });

  it('renders all primary nav items for super_admin', async () => {
    const { getByText, findByText } = render(Sidebar);
    // Wait for async profile load to populate role-gated items.
    await findByText('Users');
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

  it('hides super-admin-only nav for manage role', async () => {
    // Override the mocked profile to return a manage user.
    const apiModule = await import('../../lib/api.js');
    apiModule.api.getProfile = vi.fn(async () => ({
      data: { username: 'mgr', role: 'manage', app_access: ['x'] },
      error: null,
      status: 200,
    }));
    const { findByText, queryByText } = render(Sidebar);
    await findByText('Dashboard');
    // Allow microtasks to flush profile load.
    await tick();
    await tick();
    expect(queryByText('Users')).toBeNull();
    expect(queryByText('Registries')).toBeNull();
    expect(queryByText('Docker')).toBeNull();
    expect(queryByText('System')).toBeNull();
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
