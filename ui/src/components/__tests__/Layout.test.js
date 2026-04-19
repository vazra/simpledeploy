import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

const connectionMock = vi.hoisted(() => ({
  connected: true,
  check: vi.fn(),
  onReconnect: () => () => {},
}));

vi.mock('../../lib/stores/connection.svelte.js', () => ({ connection: connectionMock }));

import Layout from '../Layout.svelte';
import { slot } from './helpers.js';

describe('Layout', () => {
  it('renders the children slot', () => {
    connectionMock.connected = true;
    const { getByText } = render(Layout, {
      children: slot(() => `<div>page content</div>`),
    });
    expect(getByText('page content')).toBeInTheDocument();
  });

  it('shows backend-unavailable banner when connection.connected is false', () => {
    connectionMock.connected = false;
    const { getByText } = render(Layout, { children: slot('x') });
    expect(getByText('Backend unavailable')).toBeInTheDocument();
  });

  it('hides the banner when connected', () => {
    connectionMock.connected = true;
    const { queryByText } = render(Layout, { children: slot('x') });
    expect(queryByText('Backend unavailable')).toBeNull();
  });
});
