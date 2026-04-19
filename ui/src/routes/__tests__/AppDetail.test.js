import { describe, it, expect, vi } from 'vitest';
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

describe('AppDetail', () => {
  it('renders the app header from api.getApp', async () => {
    const { findByText } = render(AppDetail, { params: { slug: 'foo' } });
    expect(await findByText('foo')).toBeInTheDocument();
  });
});
