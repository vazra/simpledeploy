import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';

// the module under test imports the toasts store as a side-effect channel
import { api } from '../api.js';
import { toasts } from '../stores/toast.js';

function jsonResponse(body, status = 200, headers = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'content-type': 'application/json', ...headers }),
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function textResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'content-type': 'text/plain' }),
    json: async () => null,
    text: async () => body,
  };
}

describe('api', () => {
  let fetchMock;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    // clear any lingering toasts from previous tests
    for (const t of get(toasts)) toasts.remove(t.id);
    // reset hash so 401 redirect detection works predictably
    window.location.hash = '';
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('listApps() returns parsed JSON with no error', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([{ slug: 'foo' }]));
    const res = await api.listApps();
    expect(res.error).toBeNull();
    expect(res.data).toEqual([{ slug: 'foo' }]);
    expect(fetchMock).toHaveBeenCalledWith('/api/apps', expect.objectContaining({ method: 'GET' }));
  });

  it('sends JSON body with content-type on POST', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await api.login('u', 'p');
    const [, opts] = fetchMock.mock.calls[0];
    expect(opts.method).toBe('POST');
    expect(opts.headers['Content-Type']).toBe('application/json');
    expect(JSON.parse(opts.body)).toEqual({ username: 'u', password: 'p' });
  });

  it('propagates non-ok responses as error', async () => {
    fetchMock.mockResolvedValueOnce(textResponse('boom', 500));
    const res = await api.listApps();
    expect(res.data).toBeNull();
    expect(res.error).toBe('boom');
    expect(res.status).toBe(500);
  });

  it('falls back to "HTTP <status>" error when body is empty', async () => {
    fetchMock.mockResolvedValueOnce(textResponse('', 503));
    const res = await api.listApps();
    expect(res.error).toBe('HTTP 503');
  });

  it('redirects to /login on 401 when not already on login', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'nope' }, 401));
    const res = await api.listApps();
    expect(res.error).toBe('Unauthorized');
    expect(window.location.hash).toBe('#/login');
  });

  it('does not redirect on 401 if already on login', async () => {
    window.location.hash = '#/login';
    fetchMock.mockResolvedValueOnce(jsonResponse({}, 401));
    await api.listApps();
    expect(window.location.hash).toBe('#/login');
  });

  it('getCompose returns text payload', async () => {
    fetchMock.mockResolvedValueOnce(textResponse('services:\n  web: {}\n'));
    const res = await api.getCompose('foo');
    expect(res.data).toBe('services:\n  web: {}\n');
  });

  it('surfaces fetch errors as strings', async () => {
    fetchMock.mockRejectedValueOnce(new Error('network down'));
    const res = await api.listApps();
    expect(res.data).toBeNull();
    expect(res.error).toBe('network down');
  });

  it('requestWithToast pushes an error toast on failure', async () => {
    fetchMock.mockResolvedValueOnce(textResponse('denied', 403));
    await api.removeApp('foo');
    const t = get(toasts);
    expect(t).toHaveLength(1);
    expect(t[0].type).toBe('error');
    expect(t[0].message).toBe('denied');
  });

  it('requestWithToast pushes a success toast on success', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({}));
    await api.removeApp('foo');
    const t = get(toasts);
    expect(t).toHaveLength(1);
    expect(t[0].type).toBe('success');
    expect(t[0].message).toBe('App removed');
  });

  it('appends range query on metrics endpoints', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.systemMetrics('6h');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/metrics/system?range=6h');
  });

  it('defaults metrics range to 1h', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.appMetrics('myapp');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/myapp/metrics?range=1h');
  });

  it('downloadBackupUrl builds a URL from the run id', () => {
    expect(api.downloadBackupUrl(42)).toBe('/api/backups/runs/42/download');
  });

  it('uploadRestore returns success when upload 2xx', async () => {
    fetchMock.mockResolvedValueOnce({ ok: true, status: 200, text: async () => '' });
    const fd = new FormData();
    const res = await api.uploadRestore('foo', fd);
    expect(res.data).toBe(true);
    expect(res.error).toBeNull();
  });

  it('uploadRestore reports server text on failure', async () => {
    fetchMock.mockResolvedValueOnce({ ok: false, status: 400, text: async () => 'bad file' });
    const res = await api.uploadRestore('foo', new FormData());
    expect(res.data).toBeNull();
    expect(res.error).toBe('bad file');
  });

  it('deployLogsWs returns a WebSocket with correct URL', () => {
    const origWS = global.WebSocket;
    const ctor = vi.fn();
    class FakeWS {
      constructor(url) { ctor(url); }
    }
    global.WebSocket = FakeWS;
    try {
      api.deployLogsWs('myapp');
      expect(ctor).toHaveBeenCalledTimes(1);
      const url = ctor.mock.calls[0][0];
      expect(url).toMatch(/^wss?:\/\/.+\/api\/apps\/myapp\/deploy-logs$/);
    } finally {
      global.WebSocket = origWS;
    }
  });

  it('systemLogsWs returns a WebSocket pointing at process-logs/stream', () => {
    const origWS = global.WebSocket;
    const ctor = vi.fn();
    class FakeWS { constructor(url) { ctor(url); } }
    global.WebSocket = FakeWS;
    try {
      api.systemLogsWs();
      const url = ctor.mock.calls[0][0];
      expect(url).toMatch(/\/api\/system\/process-logs\/stream$/);
    } finally {
      global.WebSocket = origWS;
    }
  });

  it('encodes URL components for cert and docker endpoints', async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    await api.deleteCert('foo', 'sub.example.com');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/foo/certs/sub.example.com');

    fetchMock.mockClear();
    await api.dockerRemoveImage('sha256:abc/def');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/docker/images/sha256%3Aabc%2Fdef');
  });
});
