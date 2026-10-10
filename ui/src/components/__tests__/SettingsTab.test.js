import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';

// Uses the real api.js over a stubbed fetch so refusal reasons travel the
// same path as in production (baseRequest -> requestWithToast -> toast).
vi.mock('svelte-spa-router', () => ({
  push: vi.fn(),
  default: null,
}));

import SettingsTab from '../SettingsTab.svelte';
import { toasts } from '../../lib/stores/toast.js';

function json(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'content-type': 'application/json' }),
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function text(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'content-type': 'text/plain' }),
    json: async () => null,
    text: async () => body,
  };
}

const refusal = {
  error: "this app's compose file no longer passes security checks; fix it and redeploy before changing these settings",
  violations: ['service "web": privileged not allowed'],
};
const refusalText = "this app's compose file no longer passes security checks; fix it and redeploy before changing these settings:\n- service \"web\": privileged not allowed";

let routes;
let fetchMock;

function installFetch() {
  fetchMock = vi.fn(async (url, opts = {}) => {
    const key = `${opts.method || 'GET'} ${url}`;
    const handler = routes[key];
    if (handler) return handler();
    if (key === 'GET /api/apps/foo/compose') return text('services:\n  web:\n    image: nginx\n');
    if (key === 'GET /api/apps/foo/env') return json([]);
    if (key === 'GET /api/apps/foo/versions') return json([]);
    if (key === 'GET /api/system/public-host') return json({ public_host: '' });
    return json({});
  });
  vi.stubGlobal('fetch', fetchMock);
}

const baseApp = {
  Name: 'foo',
  Slug: 'foo',
  Labels: {},
  ComposePath: '/srv/apps/foo/docker-compose.yml',
  CreatedAt: '2024-01-01T00:00:00Z',
};

function renderTab(props = {}) {
  return render(SettingsTab, {
    slug: 'foo',
    app: baseApp,
    services: [{ service: 'web' }],
    onAppUpdated: () => {},
    ...props,
  });
}

function calls(method, url) {
  return fetchMock.mock.calls.filter(([u, o = {}]) => u === url && (o.method || 'GET') === method);
}

describe('SettingsTab', () => {
  beforeEach(() => {
    routes = {};
    installFetch();
    for (const t of get(toasts)) toasts.remove(t.id);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders without crashing', () => {
    const { container } = renderTab();
    expect(container.firstChild).not.toBeNull();
  });

  it('exposes Danger Zone / advanced sections', () => {
    const { container } = renderTab({ services: [] });
    expect(container.textContent).toMatch(/Danger|Advanced|Remove/i);
  });

  it('expands Advanced section without throwing when ComposePath is set', async () => {
    const { getByRole, container } = renderTab({ services: [] });
    await fireEvent.click(getByRole('button', { name: /Advanced/i }));
    expect(container.textContent).toMatch(/IP Allowlist/);
    expect(container.textContent).toMatch(/\.env/);
  });

  it('keeps the new endpoint form and input when saving fails, and shows the reasons', async () => {
    routes['PUT /api/apps/foo/endpoints'] = () => json(refusal, 409);
    const onAppUpdated = vi.fn();
    const { getByText, findByText, container } = renderTab({ onAppUpdated });
    await fireEvent.click(getByText('+ Add'));
    const domain = container.querySelector('input[placeholder="myapp.example.com"]');
    await fireEvent.input(domain, { target: { value: 'shop.example.com' } });
    await fireEvent.input(container.querySelector('input[placeholder="3000"]'), { target: { value: '8080' } });
    await fireEvent.click(await findByText('Add Endpoint'));

    await waitFor(() => expect(calls('PUT', '/api/apps/foo/endpoints')).toHaveLength(1));
    await waitFor(() => expect(get(toasts).some((t) => t.type === 'error' && t.message === refusalText)).toBe(true));
    // form still open, with what the user typed
    expect(getByText('Add Endpoint')).toBeInTheDocument();
    expect(container.querySelector('input[placeholder="myapp.example.com"]').value).toBe('shop.example.com');
    expect(onAppUpdated).not.toHaveBeenCalled();
  });

  it('keeps editing an existing endpoint when saving fails (domain conflict)', async () => {
    routes['PUT /api/apps/foo/endpoints'] = () =>
      text('endpoint 0: domain taken.example.com is already used by app "bar"; remove it there first or pick another domain\n', 409);
    const app = { ...baseApp, endpoints: [{ domain: 'old.example.com', service: 'web', port: '80', tls: 'letsencrypt' }] };
    const { getByTitle, getByText, container } = renderTab({ app });
    await fireEvent.click(getByTitle('Edit'));
    const domain = container.querySelector('input[placeholder="myapp.example.com"]');
    await fireEvent.input(domain, { target: { value: 'taken.example.com' } });
    await fireEvent.click(getByText('Save'));

    await waitFor(() => expect(get(toasts).some((t) => t.type === 'error' && /already used by app "bar"/.test(t.message))).toBe(true));
    expect(container.querySelector('input[placeholder="myapp.example.com"]').value).toBe('taken.example.com');
  });

  it('closes the endpoint form after a successful save', async () => {
    routes['PUT /api/apps/foo/endpoints'] = () => json({ status: 'ok' });
    const onAppUpdated = vi.fn();
    const { getByText, queryByText, container } = renderTab({ onAppUpdated });
    await fireEvent.click(getByText('+ Add'));
    await fireEvent.input(container.querySelector('input[placeholder="myapp.example.com"]'), { target: { value: 'ok.example.com' } });
    await fireEvent.click(getByText('Add Endpoint'));
    await waitFor(() => expect(queryByText('Add Endpoint')).toBeNull());
    expect(onAppUpdated).toHaveBeenCalled();
  });

  async function openCustomCertForm() {
    const app = { ...baseApp, endpoints: [{ domain: 'secure.example.com', service: 'web', port: '443', tls: 'custom' }] };
    const r = renderTab({ app });
    await fireEvent.click(r.getByTitle('Edit'));
    const certInput = r.getByPlaceholderText('-----BEGIN CERTIFICATE-----');
    const keyInput = r.getByPlaceholderText('-----BEGIN PRIVATE KEY-----');
    await fireEvent.input(certInput, { target: { value: 'CERT-PEM' } });
    await fireEvent.input(keyInput, { target: { value: 'KEY-PEM' } });
    return { ...r, certInput, keyInput };
  }

  it('keeps the pasted certificate and key when the upload is refused, and shows the error', async () => {
    routes['PUT /api/apps/foo/certs/secure.example.com'] = () =>
      json({ error: 'certificate and key do not match' }, 400);
    const { getByText, certInput, keyInput } = await openCustomCertForm();
    await fireEvent.click(getByText('Upload Certificate'));

    await waitFor(() => expect(calls('PUT', '/api/apps/foo/certs/secure.example.com')).toHaveLength(1));
    expect(JSON.parse(calls('PUT', '/api/apps/foo/certs/secure.example.com')[0][1].body)).toEqual({ cert: 'CERT-PEM', key: 'KEY-PEM' });
    await waitFor(() => expect(get(toasts).some((t) => t.type === 'error' && /do not match/.test(t.message))).toBe(true));
    await waitFor(() => expect(getByText('Upload Certificate')).toBeInTheDocument());
    expect(certInput.value).toBe('CERT-PEM');
    expect(keyInput.value).toBe('KEY-PEM');
    expect(get(toasts).some((t) => t.type === 'success')).toBe(false);
  });

  it('clears the pasted certificate and key after a successful upload', async () => {
    routes['PUT /api/apps/foo/certs/secure.example.com'] = () => json({ status: 'ok' });
    const { getByText, certInput, keyInput } = await openCustomCertForm();
    await fireEvent.click(getByText('Upload Certificate'));

    await waitFor(() => expect(get(toasts).some((t) => t.type === 'success' && t.message === 'Certificate uploaded')).toBe(true));
    await waitFor(() => expect(certInput.value).toBe(''));
    expect(keyInput.value).toBe('');
  });

  it('shows refusal reasons when the IP allowlist save is refused', async () => {
    routes['PUT /api/apps/foo/access'] = () => json(refusal, 409);
    const { getByRole, getByLabelText, findByText } = renderTab();
    await fireEvent.click(getByRole('button', { name: /Advanced/i }));
    await fireEvent.input(getByLabelText('IP Allowlist'), { target: { value: '10.0.0.0/8' } });
    await fireEvent.click(await findByText('Save'));
    await waitFor(() => expect(get(toasts).some((t) => t.type === 'error' && t.message === refusalText)).toBe(true));
    expect(getByLabelText('IP Allowlist').value).toBe('10.0.0.0/8');
  });

  it('gives viewers a read-only environment editor', async () => {
    const { findByTestId, queryByText } = renderTab({ canMutate: false });
    expect(await findByTestId('env-readonly-hint')).toBeInTheDocument();
    expect(queryByText('Add Variable')).toBeNull();
  });

  it('lets users who can change the app edit environment variables', async () => {
    const { findByText, queryByTestId } = renderTab({ canMutate: true });
    expect(await findByText('Add Variable')).toBeInTheDocument();
    expect(queryByTestId('env-readonly-hint')).toBeNull();
  });
});
