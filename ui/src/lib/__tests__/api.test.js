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

  it('listActivity builds URL with categories and limit', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.listActivity({ categories: ['compose'], limit: 10 });
    const url = fetchMock.mock.calls[0][0];
    expect(url).toContain('categories=compose');
    expect(url).toContain('limit=10');
    expect(url).toMatch(/^\/api\/activity\?/);
  });

  it('listActivity omits app and before when not set', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.listActivity({ limit: 5 });
    const url = fetchMock.mock.calls[0][0];
    expect(url).not.toContain('app=');
    expect(url).not.toContain('before=');
    expect(url).toContain('limit=5');
  });

  it('listActivity includes app and before when set', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.listActivity({ app: 'myapp', before: 99, limit: 20 });
    const url = fetchMock.mock.calls[0][0];
    expect(url).toContain('app=myapp');
    expect(url).toContain('before=99');
    expect(url).toContain('limit=20');
  });

  it('listAppActivity builds URL for a specific app', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.listAppActivity('my-app', { categories: ['deploy', 'backup'], limit: 25 });
    const url = fetchMock.mock.calls[0][0];
    expect(url).toBe('/api/apps/my-app/activity?categories=deploy%2Cbackup&limit=25');
  });

  it('listRecentActivity uses default limit of 8', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.listRecentActivity();
    expect(fetchMock.mock.calls[0][0]).toBe('/api/activity/recent?limit=8');
  });

  it('listRecentActivity accepts a custom limit', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    await api.listRecentActivity(15);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/activity/recent?limit=15');
  });

  it('getActivity fetches a single activity entry by id', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ id: 7 }));
    const res = await api.getActivity(7);
    expect(res.data).toEqual({ id: 7 });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/activity/7');
  });

  it('purgeActivity sends DELETE to /activity', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({}));
    await api.purgeActivity();
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/activity');
    expect(opts.method).toBe('DELETE');
  });

  it('getAuditConfig GET /system/audit-config', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ retention_days: 30 }));
    const res = await api.getAuditConfig();
    expect(res.data).toEqual({ retention_days: 30 });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/system/audit-config');
  });

  it('putAuditConfig PUT /system/audit-config with body', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({}));
    await api.putAuditConfig({ retention_days: 60 });
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/system/audit-config');
    expect(opts.method).toBe('PUT');
    expect(JSON.parse(opts.body)).toEqual({ retention_days: 60 });
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
