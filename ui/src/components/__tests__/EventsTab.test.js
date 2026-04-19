import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  getDeployEvents: vi.fn(async () => ({
    data: [
      { id: 1, action: 'deploy', created_at: '2026-01-01T00:00:00Z' },
      { id: 2, action: 'restart', created_at: '2026-01-02T00:00:00Z' },
    ],
    error: null,
  })),
  deployLogsWs: vi.fn(() => ({ onmessage: null, onclose: null, close: () => {} })),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import EventsTab from '../EventsTab.svelte';

describe('EventsTab', () => {
  it('loads events on mount', async () => {
    render(EventsTab, { slug: 'foo' });
    await waitFor(() => expect(apiMock.getDeployEvents).toHaveBeenCalledWith('foo'));
  });

  it('renders event rows', async () => {
    const { container } = render(EventsTab, { slug: 'foo' });
    await waitFor(() => {
      expect(container.textContent.toLowerCase()).toMatch(/deploy|restart/);
    });
  });

  it('connects the websocket when deploying becomes true', async () => {
    const { rerender } = render(EventsTab, { slug: 'foo', deploying: false });
    expect(apiMock.deployLogsWs).not.toHaveBeenCalled();
    await rerender({ slug: 'foo', deploying: true });
    await waitFor(() => expect(apiMock.deployLogsWs).toHaveBeenCalledWith('foo'));
  });
});
