import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      systemInfo: vi.fn(async () => ({
        data: {
          simpledeploy: { version: '1.2.3', deployment_mode: 'native', deployment_label: 'Native', process: { mem_alloc: 1024 * 1024 } },
          database: { size_bytes: 1024 },
        },
        error: null,
      })),
      systemStorageBreakdown: vi.fn(async () => ({ data: {}, error: null })),
      systemAuditLog: vi.fn(async () => ({ data: [], error: null })),
      systemAuditConfig: vi.fn(async () => ({ data: { max_size: 1000 }, error: null })),
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import System from '../System.svelte';

describe('System route', () => {
  it('renders system info with version', async () => {
    const { findByText } = render(System);
    expect(await findByText(/1\.2\.3/)).toBeInTheDocument();
  });
});
