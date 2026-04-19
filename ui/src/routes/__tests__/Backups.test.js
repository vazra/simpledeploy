import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      backupSummary: vi.fn(async () => ({
        data: { apps: [] },
        error: null,
      })),
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Backups from '../Backups.svelte';

describe('Backups route', () => {
  it('renders without crashing and shows the Backups heading', async () => {
    const { findAllByText } = render(Backups);
    await waitFor(async () => {
      const nodes = await findAllByText(/Backups|backup/i);
      expect(nodes.length).toBeGreaterThan(0);
    });
  });
});
