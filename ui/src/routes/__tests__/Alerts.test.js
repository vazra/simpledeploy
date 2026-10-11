import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, waitFor, fireEvent, screen } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({}));

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  Object.assign(apiMock, makeApiMock({
    listWebhooks: vi.fn(async () => ({ data: [], error: null })),
    listAlertRules: vi.fn(async () => ({ data: [], error: null })),
    alertHistory: vi.fn(async () => ({ data: [], error: null })),
  }));
  return { api: apiMock };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Alerts from '../Alerts.svelte';

const ok = (data) => ({ data, error: null });

function selectWithOption(text) {
  return screen.getByRole('option', { name: text }).closest('select');
}

async function choose(select, value) {
  select.value = value;
  await fireEvent.change(select);
}

describe('Alerts route', () => {
  it('renders without crashing and mentions Alerts/webhooks', async () => {
    const { container } = render(Alerts);
    await waitFor(() => expect(container.textContent.length).toBeGreaterThan(0));
    expect(container.textContent.toLowerCase()).toMatch(/alert|webhook|rule/);
  });

  describe('rule form payload', () => {
    const rule = {
      id: 9, app_id: 1, app_slug: 'alpha', metric: 'cpu_pct', operator: '>',
      threshold: 80, duration_sec: 60, webhook_id: 5, enabled: true,
    };

    beforeEach(() => {
      apiMock.listApps.mockResolvedValue(ok([{ ID: 1, Slug: 'alpha' }, { ID: 2, Slug: 'beta' }]));
      apiMock.listWebhooks.mockResolvedValue(ok([{ id: 5, name: 'Ops', type: 'slack' }]));
      apiMock.listAlertRules.mockResolvedValue(ok([rule]));
      apiMock.createAlertRule.mockClear();
      apiMock.updateAlertRule.mockClear();
    });

    it('sends the chosen app when creating a rule', async () => {
      render(Alerts);
      await fireEvent.click(await screen.findByText('Add Rule'));
      await choose(selectWithOption('All Apps'), 'beta');
      await choose(selectWithOption('Select webhook'), '5');
      await fireEvent.click(screen.getByRole('button', { name: 'Create' }));
      await waitFor(() => expect(apiMock.createAlertRule).toHaveBeenCalledTimes(1));
      expect(apiMock.createAlertRule.mock.calls[0][0]).toMatchObject({
        app_slug: 'beta', metric: 'cpu_pct', webhook_id: 5, enabled: true,
      });
    });

    it('sends an empty app for an all-apps rule', async () => {
      render(Alerts);
      await fireEvent.click(await screen.findByText('Add Rule'));
      await choose(selectWithOption('Select webhook'), '5');
      await fireEvent.click(screen.getByRole('button', { name: 'Create' }));
      await waitFor(() => expect(apiMock.createAlertRule).toHaveBeenCalledTimes(1));
      expect(apiMock.createAlertRule.mock.calls[0][0].app_slug).toBe('');
    });

    it('keeps the rule app when editing a rule', async () => {
      render(Alerts);
      await fireEvent.click(await screen.findByText(/CPU Usage > 80\.0% - alpha/));
      expect(selectWithOption('All Apps').value).toBe('alpha');
      await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await waitFor(() => expect(apiMock.updateAlertRule).toHaveBeenCalledTimes(1));
      const [id, payload] = apiMock.updateAlertRule.mock.calls[0];
      expect(id).toBe(9);
      expect(payload).toMatchObject({ app_slug: 'alpha', webhook_id: 5 });
    });
  });
});
