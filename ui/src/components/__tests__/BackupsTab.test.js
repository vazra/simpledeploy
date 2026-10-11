import { describe, it, expect, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  listBackupConfigs: vi.fn(async () => ({ data: [], error: null })),
  listBackupRuns: vi.fn(async () => ({ data: [], error: null })),
  detectStrategies: vi.fn(async () => ({ data: [], error: null })),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));
const realtimeMock = vi.hoisted(() => ({
  handlers: {},
  register: vi.fn((topic, fn) => {
    realtimeMock.handlers[topic] = fn;
    return () => {};
  }),
}));
vi.mock('../../lib/stores/realtime.svelte.js', () => ({ realtime: realtimeMock }));

import BackupsTab from '../BackupsTab.svelte';

describe('BackupsTab', () => {
  it('fetches configs and runs on mount', async () => {
    render(BackupsTab, { slug: 'foo' });
    await waitFor(() => {
      expect(apiMock.listBackupConfigs).toHaveBeenCalledWith('foo');
      expect(apiMock.listBackupRuns).toHaveBeenCalledWith('foo');
    });
  });

  // Runs publish on app:<slug> when they start and finish.
  it('refetches runs when the app topic fires', async () => {
    render(BackupsTab, { slug: 'foo' });
    await waitFor(() => expect(realtimeMock.register).toHaveBeenCalledWith('app:foo', expect.any(Function)));
    const before = apiMock.listBackupRuns.mock.calls.length;
    apiMock.listBackupRuns.mockResolvedValueOnce({ data: [{ id: 5, status: 'success', started_at: new Date().toISOString() }], error: null });
    await realtimeMock.handlers['app:foo']();
    expect(apiMock.listBackupRuns.mock.calls.length).toBe(before + 1);
  });

  it('renders without crashing when there are no configs', async () => {
    const { container } = render(BackupsTab, { slug: 'foo' });
    await waitFor(() => expect(container.firstChild).not.toBeNull());
  });

  // The server stores time-based retention as "time".
  it('shows days retention for configs stored with mode time', async () => {
    apiMock.listBackupConfigs.mockResolvedValueOnce({
      data: [{ id: 1, strategy: 'postgres', target: 'local', schedule_cron: '0 2 * * *', retention_mode: 'time', retention_days: 14, retention_count: 0 }],
      error: null,
    });
    const { findByText } = render(BackupsTab, { slug: 'foo' });
    expect(await findByText('14 days')).toBeTruthy();
  });

  it('labels the upload-restore target as an optional service name', async () => {
    apiMock.listBackupConfigs.mockResolvedValueOnce({ data: [{ id: 1, strategy: 'postgres' }], error: null });
    const { findByText, getByLabelText, getByTestId } = render(BackupsTab, { slug: 'foo' });
    await fireEvent.click(await findByText('Restore from File'));
    const input = getByLabelText('Service name (optional)');
    expect(input.getAttribute('type')).toBe('text');
    expect(input.getAttribute('placeholder')).toBe('e.g. db');
    expect(getByTestId('upload-restore-service-hint')).toHaveTextContent(/single container/i);
  });
});
