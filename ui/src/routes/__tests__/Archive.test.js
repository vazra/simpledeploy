import { describe, it, expect, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/svelte';

const purgeApp = vi.fn(async () => ({ data: {}, error: null }));
const listArchived = vi.fn();

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      listArchived: (...a) => listArchived(...a),
      purgeApp: (...a) => purgeApp(...a),
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Archive from '../Archive.svelte';

describe('Archive route', () => {
  it('shows empty state when no archived apps', async () => {
    listArchived.mockResolvedValueOnce({ data: [], error: null });
    const { findByText } = render(Archive);
    expect(await findByText('No archived apps.')).toBeInTheDocument();
  });

  it('renders archived row and expands tombstone details', async () => {
    listArchived.mockResolvedValueOnce({
      data: [{
        slug: 'old-app',
        display_name: 'Old App',
        domain: 'old.example.com',
        archived_at: new Date().toISOString(),
        tombstone: {
          version: 1,
          archived_at: new Date().toISOString(),
          app: { slug: 'old-app', display_name: 'Old App' },
          alert_rules: [{ name: 'cpu-high' }],
          backup_configs: [{ name: 'nightly' }],
          access: [{ username: 'alice', role: 'admin' }],
        },
      }],
      error: null,
    });
    const { findByText, getByText } = render(Archive);
    expect(await findByText('Old App')).toBeInTheDocument();
    expect(getByText('old.example.com')).toBeInTheDocument();
    await fireEvent.click(getByText('Details'));
    await waitFor(() => expect(getByText('cpu-high')).toBeInTheDocument());
    expect(getByText('nightly')).toBeInTheDocument();
    expect(getByText('alice (admin)')).toBeInTheDocument();
  });

  it('purges via confirm modal', async () => {
    listArchived.mockResolvedValueOnce({
      data: [{ slug: 'gone', display_name: 'Gone', archived_at: new Date().toISOString(), tombstone: null }],
      error: null,
    });
    listArchived.mockResolvedValueOnce({ data: [], error: null });
    const { findByText, getByText } = render(Archive);
    await findByText('Gone');
    await fireEvent.click(getByText('Clean up'));
    await waitFor(() => expect(getByText(/Permanently clean up/)).toBeInTheDocument());
    await fireEvent.click(getByText('Confirm'));
    await waitFor(() => expect(purgeApp).toHaveBeenCalledWith('gone'));
    expect(purgeApp).toHaveBeenCalledTimes(1);
  });
});
