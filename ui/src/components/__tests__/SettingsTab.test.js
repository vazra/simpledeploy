import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

vi.mock('svelte-spa-router', () => ({
  push: vi.fn(),
  default: null,
}));

import SettingsTab from '../SettingsTab.svelte';

describe('SettingsTab (smoke)', () => {
  it('renders without crashing', () => {
    const { container } = render(SettingsTab, {
      slug: 'foo',
      app: { Name: 'foo', Labels: {} },
      services: [{ service: 'web' }],
      onAppUpdated: () => {},
    });
    expect(container.firstChild).not.toBeNull();
  });

  it('exposes Danger Zone / advanced sections', () => {
    const { container } = render(SettingsTab, {
      slug: 'foo',
      app: { Name: 'foo', Labels: {} },
      services: [],
      onAppUpdated: () => {},
    });
    expect(container.textContent).toMatch(/Danger|Advanced|Remove/i);
  });

  it('expands Advanced section without throwing when ComposePath is set', async () => {
    const { getByRole, container } = render(SettingsTab, {
      slug: 'foo',
      app: {
        Name: 'foo',
        Slug: 'foo',
        Labels: {},
        ComposePath: '/srv/apps/foo/docker-compose.yml',
        CreatedAt: '2024-01-01T00:00:00Z',
      },
      services: [],
      onAppUpdated: () => {},
    });
    const btn = getByRole('button', { name: /Advanced/i });
    await fireEvent.click(btn);
    expect(container.textContent).toMatch(/IP Allowlist/);
    expect(container.textContent).toMatch(/\.env/);
  });
});
