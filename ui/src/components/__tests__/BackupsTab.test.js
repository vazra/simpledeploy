import { describe, it, expect, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  listBackupConfigs: vi.fn(async () => ({ data: [], error: null })),
  listBackupRuns: vi.fn(async () => ({ data: [], error: null })),
  detectStrategies: vi.fn(async () => ({ data: [], error: null })),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import BackupsTab from '../BackupsTab.svelte';

describe('BackupsTab', () => {
  it('fetches configs and runs on mount', async () => {
    render(BackupsTab, { slug: 'foo' });
    await waitFor(() => {
      expect(apiMock.listBackupConfigs).toHaveBeenCalledWith('foo');
      expect(apiMock.listBackupRuns).toHaveBeenCalledWith('foo');
    });
  });

  it('renders without crashing when there are no configs', async () => {
    const { container } = render(BackupsTab, { slug: 'foo' });
    await waitFor(() => expect(container.firstChild).not.toBeNull());
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
