import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor, within } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import VisualEditor from '../VisualEditor.svelte';
import { api } from '../../lib/api.js';

describe('VisualEditor (smoke)', () => {
  it('renders with an empty compose without crashing', () => {
    const { container } = render(VisualEditor, { compose: { services: {} }, slug: '' });
    expect(container.firstChild).not.toBeNull();
  });

  it('renders with a minimal single-service compose', () => {
    const compose = {
      services: {
        web: {
          image: 'nginx:alpine',
          labels: {
            'simpledeploy.endpoints.0.domain': 'example.com',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
          },
        },
      },
    };
    const { container } = render(VisualEditor, { compose, slug: 'foo' });
    expect(container.textContent).toMatch(/nginx/);
  });

  it('renders some accordion section buttons for a populated compose', () => {
    const compose = {
      services: {
        api: {
          image: 'caddy:latest',
          labels: {
            'simpledeploy.endpoints.0.domain': 'api.example.com',
            'simpledeploy.endpoints.0.port': '8080',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
          },
        },
      },
    };
    const { container } = render(VisualEditor, { compose, slug: 'foo' });
    // Accordion headers are rendered as buttons even when sections are collapsed.
    expect(container.querySelectorAll('button').length).toBeGreaterThan(0);
  });

  it('shows a placeholder for masked .env values instead of empty', async () => {
    api.getEnv.mockResolvedValueOnce({ data: [{ key: 'SECRET', value: '', masked: true }], error: null });
    const compose = { services: { web: { image: 'nginx', environment: { SECRET: '${SECRET}' } } } };
    const { findByTitle, findByText } = render(VisualEditor, { compose, slug: 'foo' });
    await waitFor(() => expect(api.getEnv).toHaveBeenCalledWith('foo'));
    await fireEvent.click(await findByTitle('Edit web'));
    await fireEvent.click(await findByTitle('Show .env value'));
    expect(await findByText('•••••• (hidden for viewers)')).toBeInTheDocument();
  });

  describe('.env actions follow the canEdit permission', () => {
    const compose = { services: { web: { image: 'nginx', environment: { FOO: '' } } } };

    beforeEach(() => {
      api.putEnv.mockClear();
    });

    it('shows "Use from .env" and the add-variable form for editors', async () => {
      api.getEnv.mockResolvedValueOnce({ data: [{ key: 'FOO', value: 'bar' }], error: null });
      const { findByTitle, findByText, getByText } = render(VisualEditor, { compose, slug: 'foo', canEdit: true });
      await fireEvent.click(await findByTitle('Edit web'));
      await fireEvent.click(await findByText('Use from .env'));
      expect(getByText('Add new variable')).toBeInTheDocument();
    });

    it('hides "Use from .env" for users who cannot edit', async () => {
      api.getEnv.mockResolvedValueOnce({ data: [{ key: 'FOO', value: '' }], error: null });
      const { findByTitle, queryByText } = render(VisualEditor, { compose, slug: 'foo', canEdit: false });
      await fireEvent.click(await findByTitle('Edit web'));
      expect(queryByText('Use from .env')).toBeNull();
    });

    it('defaults to no .env actions when canEdit is not passed', async () => {
      const { findByTitle, queryByText } = render(VisualEditor, { compose, slug: 'foo' });
      await fireEvent.click(await findByTitle('Edit web'));
      expect(queryByText('Use from .env')).toBeNull();
    });

    async function openPickerAndAdd(onchange) {
      api.getEnv.mockResolvedValueOnce({ data: [{ key: 'FOO', value: 'bar' }], error: null });
      const r = render(VisualEditor, { compose, slug: 'foo', canEdit: true, onchange });
      await waitFor(() => expect(api.getEnv).toHaveBeenCalledWith('foo'));
      await fireEvent.click(await r.findByTitle('Edit web'));
      await fireEvent.click(await r.findByText('Use from .env'));
      const dialog = within(r.getByRole('dialog'));
      await fireEvent.input(dialog.getByPlaceholderText('KEY'), { target: { value: 'NEW_KEY' } });
      await fireEvent.input(dialog.getByPlaceholderText('value'), { target: { value: 'secret' } });
      await fireEvent.click(dialog.getByText('Add & Use'));
      return { ...r, dialog };
    }

    it('does not reference a new variable in compose when the .env save is refused', async () => {
      api.putEnv.mockResolvedValueOnce({ data: null, error: 'refused' });
      const onchange = vi.fn();
      const { dialog } = await openPickerAndAdd(onchange);

      await waitFor(() => expect(api.putEnv).toHaveBeenCalledTimes(1));
      expect(api.putEnv).toHaveBeenCalledWith('foo', [{ key: 'FOO', value: 'bar' }, { key: 'NEW_KEY', value: 'secret' }]);
      // Picker stays open with the typed input; the variable was rolled back.
      await waitFor(() => expect(dialog.getByText('Add & Use')).not.toBeDisabled());
      expect(dialog.getByPlaceholderText('KEY').value).toBe('NEW_KEY');
      expect(dialog.getByText('FOO')).toBeInTheDocument();
      expect(dialog.queryByText('NEW_KEY')).toBeNull();
      expect(onchange).not.toHaveBeenCalled();
    });

    it('references the new variable in compose once the .env save succeeds', async () => {
      api.putEnv.mockResolvedValueOnce({ data: {}, error: null });
      const onchange = vi.fn();
      const { queryByRole } = await openPickerAndAdd(onchange);

      await waitFor(() => expect(onchange).toHaveBeenCalled());
      expect(onchange.mock.calls.at(-1)[0].services.web.environment).toEqual({ FOO: '${NEW_KEY}' });
      expect(queryByRole('dialog')).toBeNull();
    });

    it('never offers to write .env when values are masked, even for editors', async () => {
      api.getEnv.mockResolvedValueOnce({ data: [{ key: 'SECRET', value: '', masked: true }], error: null });
      const { findByTitle, findByText, queryByText } = render(VisualEditor, { compose, slug: 'foo', canEdit: true });
      await waitFor(() => expect(api.getEnv).toHaveBeenCalledWith('foo'));
      await fireEvent.click(await findByTitle('Edit web'));
      await fireEvent.click(await findByText('Use from .env'));
      expect(queryByText('Add new variable')).toBeNull();
      expect(api.putEnv).not.toHaveBeenCalled();
    });
  });
});
