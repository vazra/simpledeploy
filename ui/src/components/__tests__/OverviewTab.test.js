import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  appRequests: vi.fn(async () => ({ data: { points: [{ n: 10, e: 1 }, { n: 20, e: 1 }] }, error: null })),
  appMetrics: vi.fn(async () => ({ data: { points: [{ cpu: 5 }] }, error: null })),
  getDeployEvents: vi.fn(async () => ({ data: [{ event: 'deployed', at: '2026-01-01T00:00:00Z' }], error: null })),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import OverviewTab from '../OverviewTab.svelte';

describe('OverviewTab', () => {
  it('fetches app metrics, requests and events on mount', async () => {
    render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo', Status: 'running' },
      services: [{ service: 'web' }],
    });
    await waitFor(() => {
      expect(apiMock.appRequests).toHaveBeenCalledWith('foo', '1h');
      expect(apiMock.appMetrics).toHaveBeenCalledWith('foo', '1h');
      expect(apiMock.getDeployEvents).toHaveBeenCalledWith('foo');
    });
  });

  it('renders total requests and error-rate stats', async () => {
    const { findByText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo', Status: 'running' },
      services: [{ service: 'web' }],
    });
    expect(await findByText(/30/)).toBeInTheDocument();
  });
});
