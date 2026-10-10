import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      getApp: vi.fn(async () => ({
        data: { Name: 'foo', Slug: 'foo', Status: 'running', Domain: 'foo.example.com', Labels: {} },
        error: null,
      })),
      getAppServices: vi.fn(async () => ({ data: [{ service: 'web' }], error: null })),
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

import AppDetail from '../AppDetail.svelte';
import { api } from '../../lib/api.js';

describe('AppDetail', () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it('renders the app header from api.getApp', async () => {
    const { findByText } = render(AppDetail, { params: { slug: 'foo' } });
    expect(await findByText('foo')).toBeInTheDocument();
  });

  it('exposes an Events tab', async () => {
    const { findByRole } = render(AppDetail, { params: { slug: 'foo' } });
    expect(await findByRole('button', { name: /^events$/i })).toBeInTheDocument();
  });

  it.each(['../activity?', '..', '-leading-dash', 'a/b', 'a'.repeat(64)])(
    'shows not-found and skips API calls for invalid slug %s',
    async (slug) => {
      const { findByText } = render(AppDetail, { params: { slug } });
      expect(await findByText('App not found')).toBeInTheDocument();
      expect(api.getApp).not.toHaveBeenCalled();
      expect(api.getAppServices).not.toHaveBeenCalled();
      expect(api.getProfile).not.toHaveBeenCalled();
    },
  );

  it('accepts slugs with dots, dashes and underscores', async () => {
    render(AppDetail, { params: { slug: 'my_app-1.2' } });
    await waitFor(() => expect(api.getApp).toHaveBeenCalledWith('my_app-1.2'));
  });
});
