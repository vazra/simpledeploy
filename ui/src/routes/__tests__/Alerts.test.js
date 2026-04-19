import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      listWebhooks: vi.fn(async () => ({ data: [], error: null })),
      listAlertRules: vi.fn(async () => ({ data: [], error: null })),
      alertHistory: vi.fn(async () => ({ data: [], error: null })),
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Alerts from '../Alerts.svelte';

describe('Alerts route', () => {
  it('renders without crashing and mentions Alerts/webhooks', async () => {
    const { container } = render(Alerts);
    await waitFor(() => expect(container.textContent.length).toBeGreaterThan(0));
    expect(container.textContent.toLowerCase()).toMatch(/alert|webhook|rule/);
  });
});
