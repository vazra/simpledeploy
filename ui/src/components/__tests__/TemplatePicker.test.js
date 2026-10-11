import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import TemplatePicker from '../TemplatePicker.svelte';
import {
  appTemplates,
  categories as appCategories,
  applyVars,
  applyAccessMode,
} from '../../lib/appTemplates.js';
import { parseEndpointLabels } from '../../lib/endpointLabels.js';

const templates = [
  {
    id: 'alpha',
    name: 'Alpha',
    icon: 'A',
    category: 'web',
    description: 'alpha desc',
    nameSuggestion: 'alpha',
    variables: [],
    compose: { services: {} },
  },
  {
    id: 'beta',
    name: 'Beta',
    icon: 'B',
    category: 'db',
    description: 'beta desc',
    nameSuggestion: 'beta',
    variables: [],
    compose: { services: {} },
  },
];
const categories = [
  { id: 'web', label: 'Web' },
  { id: 'db', label: 'Databases' },
];

describe('TemplatePicker (grid)', () => {
  it('lists all templates by default', () => {
    const { getByText } = render(TemplatePicker, { templates, categories });
    expect(getByText('Alpha')).toBeInTheDocument();
    expect(getByText('Beta')).toBeInTheDocument();
  });

  it('filters by search term', async () => {
    const { getByText, queryByText, getByPlaceholderText } = render(TemplatePicker, { templates, categories });
    const search = getByPlaceholderText('Search templates...');
    await fireEvent.input(search, { target: { value: 'beta' } });
    expect(getByText('Beta')).toBeInTheDocument();
    expect(queryByText('Alpha')).toBeNull();
  });

  it('filters by category chip', async () => {
    const { getByRole, getByText, queryByText } = render(TemplatePicker, { templates, categories });
    const chip = getByRole('button', { name: 'Databases' });
    await fireEvent.click(chip);
    expect(getByText('Beta')).toBeInTheDocument();
    expect(queryByText('Alpha')).toBeNull();
  });

  it('shows "No templates match" when filters exclude all', async () => {
    const { getByText, getByPlaceholderText } = render(TemplatePicker, { templates, categories });
    const search = getByPlaceholderText('Search templates...');
    await fireEvent.input(search, { target: { value: 'nothing-here' } });
    expect(getByText('No templates match.')).toBeInTheDocument();
  });

  it('fires onblank from the start-blank button', async () => {
    const onblank = vi.fn();
    const { getByText } = render(TemplatePicker, { templates, categories, onblank });
    await fireEvent.click(getByText(/Start with a blank/));
    expect(onblank).toHaveBeenCalled();
  });

  it('shows the "Browse community recipes" button in grid view', () => {
    const { getByRole } = render(TemplatePicker, { templates, categories });
    expect(getByRole('button', { name: /browse community recipes/i })).toBeInTheDocument();
  });
});

describe('TemplatePicker (quick test defaults)', () => {
  // The server rejects endpoints sharing domain + path + grpc matcher, so
  // Quick test must give every endpoint of a template its own hostname.
  it.each(appTemplates.map((t) => [t.id, t]))('%s applies with distinct endpoints', async (_id, t) => {
    const onapply = vi.fn();
    const { container, getByRole, getByPlaceholderText } = render(TemplatePicker, {
      templates: appTemplates,
      categories: appCategories,
      initialTemplateId: t.id,
      onapply,
    });
    // Wait for the public host lookup to settle on the browser fallback IP.
    await waitFor(() => expect(getByPlaceholderText(/203\.0\.113\.10/)).toHaveValue('127.0.0.1'));
    // Fill required fields that have no default (e.g. admin email).
    for (const v of t.variables || []) {
      if (!v.required || v.type === 'domain' || v.default != null || v.generate) continue;
      const input = container.querySelector(`#tpl-var-${v.key}`);
      await fireEvent.input(input, { target: { value: v.type === 'email' ? 'admin@example.com' : 'admin' } });
    }
    await fireEvent.click(getByRole('button', { name: /apply/i }));
    expect(onapply).toHaveBeenCalledTimes(1);
    const { vars, accessMode } = onapply.mock.calls[0][0];
    expect(accessMode).toBe('quick-test');
    const resolved = applyAccessMode(applyVars(t.compose, vars), accessMode);
    const eps = parseEndpointLabels(resolved.services);
    expect(eps.length).toBeGreaterThan(0);
    const keys = eps.map((e) => `${e.domain}|${e.protocol === 'grpc'}|${e.path || ''}`);
    expect(new Set(keys).size).toBe(keys.length);
    for (const e of eps) expect(e.domain).toMatch(/\.127\.0\.0\.1\.sslip\.io$/);
  });

  it('shows one auto domain per endpoint for multi-domain templates', async () => {
    const { getByLabelText, getByPlaceholderText } = render(TemplatePicker, {
      templates: appTemplates,
      categories: appCategories,
      initialTemplateId: 'minio',
    });
    await waitFor(() => expect(getByPlaceholderText(/203\.0\.113\.10/)).toHaveValue('127.0.0.1'));
    expect(getByLabelText('API domain (auto)')).toHaveValue('minio.127.0.0.1.sslip.io');
    expect(getByLabelText('Console domain (auto)')).toHaveValue('minio-console.127.0.0.1.sslip.io');
  });

  it('keeps a single "Domain (auto)" field for single-domain templates', async () => {
    const { getByLabelText, getByPlaceholderText } = render(TemplatePicker, {
      templates: appTemplates,
      categories: appCategories,
      initialTemplateId: 'uptime-kuma',
    });
    await waitFor(() => expect(getByPlaceholderText(/203\.0\.113\.10/)).toHaveValue('127.0.0.1'));
    expect(getByLabelText('Domain (auto)')).toHaveValue('uptime.127.0.0.1.sslip.io');
  });
});
