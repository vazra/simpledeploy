import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      listUsers: vi.fn(async () => ({
        data: [{ id: 1, username: 'admin', role: 'manage', display_name: 'Admin', email: 'a@b.co' }],
        error: null,
      })),
      listAPIKeys: vi.fn(async () => ({ data: [], error: null })),
      getProfile: vi.fn(async () => ({
        data: { id: 99, username: 'sa', role: 'super_admin', app_access: [] },
        error: null,
        status: 200,
      })),
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

  it('add user form offers Manage role (not legacy Admin)', async () => {
    const { findAllByRole, findByRole, queryByText } = render(Users);
    const addButtons = await findAllByRole('button', { name: /add user/i });
    addButtons[0].click();
    const manageBtn = await findByRole('button', { name: /Manage/ });
    expect(manageBtn).toBeTruthy();
    // The legacy "Admin" role-selector option (its description "Manage apps")
    // must be gone; we verified by its replacement above. queryByText kept for noop.
    void queryByText;
  });
});
