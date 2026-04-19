import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      listUsers: vi.fn(async () => ({
        data: [{ id: 1, username: 'admin', role: 'admin', display_name: 'Admin', email: 'a@b.co' }],
        error: null,
      })),
      listAPIKeys: vi.fn(async () => ({ data: [], error: null })),
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Users from '../Users.svelte';

describe('Users route', () => {
  it('renders user rows from api.listUsers', async () => {
    const { findAllByText } = render(Users);
    const tags = await findAllByText(/@admin/);
    expect(tags.length).toBeGreaterThan(0);
  });
});
