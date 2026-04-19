import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      listApps: vi.fn(async () => ({
        data: [
          { Slug: 'foo', Name: 'Foo App', Status: 'running', Domain: 'foo.example.com', Labels: {} },
        ],
        error: null,
      })),
    }),
  };
});

vi.mock('chart.js', () => ({
  Chart: vi.fn(function () { this.destroy = () => {}; this.update = () => {}; this.getDatasetMeta = () => ({ data: [] }); this.scales = { y: {} }; }),
  registerables: [],
}));
vi.mock('chartjs-adapter-date-fns', () => ({}));
vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Dashboard from '../Dashboard.svelte';

describe('Dashboard', () => {
  it('renders app cards from api.listApps', async () => {
    const { findByText } = render(Dashboard);
    expect(await findByText('Foo App')).toBeInTheDocument();
  });

  it('mounts without throwing when apps list is empty', async () => {
    const { container } = render(Dashboard);
    await waitFor(() => expect(container.firstChild).not.toBeNull());
  });
});
