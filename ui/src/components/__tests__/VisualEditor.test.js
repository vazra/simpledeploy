import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor, within } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import VisualEditor from '../VisualEditor.svelte';
import { api } from '../../lib/api.js';
import { serviceTemplates } from '../../lib/serviceTemplates.js';

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

describe('VisualEditor add service from template', () => {
  // Regression: the service was copied but its named volume was never
  // declared, so `docker compose up` failed with "undefined volume pgdata".
  it('declares the named volumes the new service references', async () => {
    const onchange = vi.fn();
    const { getByText } = render(VisualEditor, { compose: { services: {} }, slug: '', onchange });
    await fireEvent.click(getByText('Add Service'));
    await fireEvent.click(getByText('PostgreSQL'));
    expect(onchange).toHaveBeenCalledTimes(1);
    const next = onchange.mock.calls[0][0];
    expect(next.services.postgresql.volumes).toEqual(['pgdata:/var/lib/postgresql/data']);
    expect(next.volumes).toEqual({ pgdata: {} });
  });

  it('gives a second copy its own volume and keeps existing volume config', async () => {
    const onchange = vi.fn();
    const compose = {
      services: { postgresql: { image: 'postgres:16-alpine', volumes: ['pgdata:/var/lib/postgresql/data'] } },
      volumes: { pgdata: { driver: 'local' } },
    };
    const { getByText } = render(VisualEditor, { compose, slug: '', onchange });
    await fireEvent.click(getByText('Add Service'));
    await fireEvent.click(getByText('PostgreSQL'));
    const next = onchange.mock.calls[0][0];
    expect(next.services.postgresql2.volumes).toEqual(['pgdata2:/var/lib/postgresql/data']);
    expect(next.volumes).toEqual({ pgdata: { driver: 'local' }, pgdata2: {} });
  });
});

describe('VisualEditor service templates', () => {
  async function addTemplate(r, tpl) {
    await fireEvent.click(r.getByRole('button', { name: 'Add Service' }));
    const picker = within(r.getByText('Quick start from a template').closest('details'));
    await fireEvent.click(picker.getByText(tpl.name, { exact: true }).closest('button'));
  }

  // Named volume sources a template mounts (bind mounts start with / . or ~).
  function namedVolumes(config) {
    return (config.volumes || [])
      .map((v) => (typeof v === 'string' ? v.split(':')[0] : v.source))
      .filter((src) => src && !/^[/.~]/.test(src));
  }

  it('adds every template as a copy of its config, named after its id', async () => {
    const onchange = vi.fn();
    const r = render(VisualEditor, { compose: { services: { web: { image: 'nginx' } } }, slug: '', onchange });
    for (const tpl of serviceTemplates) {
      await addTemplate(r, tpl);
      const out = onchange.mock.calls.at(-1)[0];
      const name = tpl.id.replace(/-/g, '_');
      expect(Object.keys(out.services), tpl.id).toEqual(['web', name]);
      expect(out.services[name], tpl.id).toEqual(tpl.config);
      expect(out.services[name], `${tpl.id} must not share the template object`).not.toBe(tpl.config);
    }
    expect(onchange).toHaveBeenCalledTimes(serviceTemplates.length);
  });

  it('suffixes the name when a service with that name exists', async () => {
    const onchange = vi.fn();
    const compose = { services: { postgresql: { image: 'postgres:15' }, postgresql2: { image: 'postgres:14' } } };
    const r = render(VisualEditor, { compose, slug: '', onchange });
    await addTemplate(r, serviceTemplates.find((t) => t.id === 'postgresql'));
    const out = onchange.mock.calls.at(-1)[0];
    expect(Object.keys(out.services)).toEqual(['postgresql', 'postgresql2', 'postgresql3']);
    expect(out.services.postgresql).toEqual({ image: 'postgres:15' });
  });

  it('declares the named volumes a template mounts', async () => {
    const withVolumes = serviceTemplates.filter((t) => namedVolumes(t.config).length > 0);
    expect(withVolumes.length).toBeGreaterThan(0);
    const onchange = vi.fn();
    const r = render(VisualEditor, { compose: { services: {} }, slug: '', onchange });
    for (const tpl of withVolumes) {
      await addTemplate(r, tpl);
      const out = onchange.mock.calls.at(-1)[0];
      for (const vol of namedVolumes(tpl.config)) {
        expect(Object.keys(out.volumes || {}), `${tpl.id} mounts ${vol}`).toContain(vol);
      }
    }
  });
});
