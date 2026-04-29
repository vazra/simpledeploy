import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  appRequests: vi.fn(async () => ({ data: { points: [{ n: 10, e: 1 }, { n: 20, e: 1 }] }, error: null })),
  appMetrics: vi.fn(async () => ({ data: { points: [{ cpu: 5 }] }, error: null })),
  getDeployEvents: vi.fn(async () => ({ data: [{ event: 'deployed', at: '2026-01-01T00:00:00Z' }], error: null })),
  dockerInfo: vi.fn(async () => ({ data: { memory: 8 * 1024 * 1024 * 1024 }, error: null })),
  scaleApp: vi.fn(async () => ({ data: { status: 'ok' }, error: null })),
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

  it('renders restarting service with warning badge', async () => {
    const { findByText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo', Status: 'unstable' },
      services: [{ service: 'web', state: 'restarting' }],
    });
    expect(await findByText('restarting')).toBeInTheDocument();
  });

  it('shows replica +/- controls only for scalable services when canMutate', async () => {
    const { findByText, queryAllByLabelText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo' },
      services: [
        { service: 'web', state: 'running', replicas: 2, scalable: true },
        { service: 'db', state: 'running', replicas: 1, scalable: false, scale_reason: 'stateful image (postgres)' },
      ],
      canMutate: true,
    });
    await findByText('web');
    expect(queryAllByLabelText('Increase replicas').length).toBe(1);
    expect(queryAllByLabelText('Decrease replicas').length).toBe(1);
  });

  it('hides scale controls when canMutate is false', async () => {
    const { findByText, queryAllByLabelText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo' },
      services: [{ service: 'web', state: 'running', replicas: 1, scalable: true }],
      canMutate: false,
    });
    await findByText('web');
    expect(queryAllByLabelText('Increase replicas').length).toBe(0);
  });

  it('blocks scale-to-zero by disabling decrement at 1', async () => {
    const { findByLabelText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo' },
      services: [{ service: 'web', state: 'running', replicas: 1, scalable: true }],
      canMutate: true,
    });
    const dec = await findByLabelText('Decrease replicas');
    expect(dec.disabled).toBe(true);
  });

  it('calls scaleApp when increment clicked and reports server error inline', async () => {
    apiMock.scaleApp.mockResolvedValueOnce({ data: null, error: 'cannot scale db: stateful image (postgres)' });
    const { findByLabelText, findByText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo' },
      services: [{ service: 'web', state: 'running', replicas: 1, scalable: true }],
      canMutate: true,
    });
    const inc = await findByLabelText('Increase replicas');
    inc.click();
    await waitFor(() => expect(apiMock.scaleApp).toHaveBeenCalledWith('foo', { web: 2 }));
    expect(await findByText(/cannot scale/)).toBeInTheDocument();
  });

  it('humanizes deploy_unstable event label', async () => {
    apiMock.getDeployEvents.mockResolvedValueOnce({
      data: [{ action: 'deploy_unstable', at: '2026-01-01T00:00:00Z' }],
      error: null,
    });
    const { findByText } = render(OverviewTab, {
      slug: 'foo',
      app: { Name: 'foo', Status: 'unstable' },
      services: [{ service: 'web', state: 'running' }],
    });
    expect(await findByText('deployed (unstable)')).toBeInTheDocument();
  });
});
