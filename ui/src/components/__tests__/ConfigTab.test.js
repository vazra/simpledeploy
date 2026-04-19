import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      getCompose: vi.fn(async () => ({ data: 'services:\n  web:\n    image: nginx\n', error: null })),
    }),
  };
});

import ConfigTab from '../ConfigTab.svelte';

describe('ConfigTab', () => {
  it('renders the Save & Deploy button after load', async () => {
    const { findByText } = render(ConfigTab, { slug: 'foo' });
    expect(await findByText(/Save.*Deploy/i)).toBeInTheDocument();
  });

  it('renders deploy history accordion', async () => {
    const { findByText } = render(ConfigTab, { slug: 'foo' });
    expect(await findByText(/Deploy History/i)).toBeInTheDocument();
  });
});
