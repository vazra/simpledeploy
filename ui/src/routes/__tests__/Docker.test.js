import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      dockerInfo: vi.fn(async () => ({
        data: { server_version: '24.0.1', compose_version: '2.21.0', containers: 3, images: 5 },
        error: null,
      })),
      dockerImages: vi.fn(async () => ({ data: [{ Id: 'sha256:aaa', RepoTags: ['nginx:alpine'], Size: 50 * 1024 * 1024 }], error: null })),
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Docker from '../Docker.svelte';

describe('Docker route', () => {
  it('renders docker info from api', async () => {
    const { findByText } = render(Docker);
    await waitFor(async () => expect(await findByText(/24\.0\.1/)).toBeInTheDocument());
  });
});
