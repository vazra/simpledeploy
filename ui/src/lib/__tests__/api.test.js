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

  it('listUserAccess GETs /users/:id/access', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(['alpha']));
    const res = await api.listUserAccess(7);
    expect(res.data).toEqual(['alpha']);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/users/7/access');
    expect(fetchMock.mock.calls[0][1].method).toBe('GET');
  });

  it('grantUserAccess POSTs app_slug body', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ status: 'ok' }));
    await api.grantUserAccess(3, 'alpha');
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/users/3/access');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ app_slug: 'alpha' });
  });

  it('revokeUserAccess DELETEs slug-encoded URL', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ status: 'ok' }));
    await api.revokeUserAccess(3, 'app/with/slash');
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/users/3/access/app%2Fwith%2Fslash');
    expect(opts.method).toBe('DELETE');
  });

  it('encodes a traversal-style slug into a single path segment', async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    await api.getApp('../activity?');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/..%2Factivity%3F');

    fetchMock.mockClear();
    await api.getEnv('a/b#c');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/a%2Fb%23c/env');

    fetchMock.mockClear();
    await api.deleteVersion('../x', '1/../2');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/..%2Fx/versions/1%2F..%2F2');
  });

  it('encodes slug in raw fetch and URL builders', async () => {
    fetchMock.mockResolvedValue({ ok: true, status: 200, text: async () => '', blob: async () => new Blob() });
    await api.uploadRestore('../x', new FormData());
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/..%2Fx/backups/upload-restore');

    fetchMock.mockClear();
    await api.exportApp('a?b');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/a%3Fb/export');

    expect(api.downloadComposeVersionUrl('a/b', 3)).toBe('/api/apps/a%2Fb/versions/3/download');
  });

  it('rejects dot-segment slugs instead of building a traversal path', () => {
    expect(() => api.getApp('..')).toThrow(/invalid path segment/);
    expect(() => api.getApp('.')).toThrow(/invalid path segment/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('putEnv refuses to send masked (viewer) entries', async () => {
    const res = await api.putEnv('foo', [{ key: 'A', value: '1' }, { key: 'B', value: '', masked: true }]);
    expect(res.error).toMatch(/hidden/i);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(get(toasts)[0].type).toBe('error');
  });

  it('putEnv sends only key/value pairs', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ status: 'ok' }));
    await api.putEnv('foo', [{ key: 'A', value: '1', masked: false }]);
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/apps/foo/env');
    expect(JSON.parse(opts.body)).toEqual([{ key: 'A', value: '1' }]);
  });

  it('encodes URL components for cert and docker endpoints', async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    await api.deleteCert('foo', 'sub.example.com');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/apps/foo/certs/sub.example.com');

    fetchMock.mockClear();
    await api.dockerRemoveImage('sha256:abc/def');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/docker/images/sha256%3Aabc%2Fdef');
  });

  describe('refusal reasons (violations)', () => {
    const refused = {
      error: 'compose file contains disallowed directives',
      violations: ['service "web": privileged not allowed', 'service "web": pid "host" not allowed'],
    };
    const expected = 'compose file contains disallowed directives:\n- service "web": privileged not allowed\n- service "web": pid "host" not allowed';

    it('deploy returns the violations in error and alongside', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse(refused, 400));
      const res = await api.deploy('foo', 'Zm9v', 'update', true);
      expect(res.error).toBe(expected);
      expect(res.violations).toEqual(refused.violations);
      expect(res.status).toBe(400);
    });

    it('endpoint save toasts the reasons on 409', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'compose fails checks', violations: ['volume "x": bind not allowed'] }, 409));
      const res = await api.updateEndpoints('foo', []);
      expect(res.status).toBe(409);
      const t = get(toasts);
      expect(t).toHaveLength(1);
      expect(t[0].type).toBe('error');
      expect(t[0].message).toBe('compose fails checks:\n- volume "x": bind not allowed');
    });

    it('rollback and version restore toast the reasons', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse(refused, 400));
      await api.rollbackApp('foo', 3);
      fetchMock.mockResolvedValueOnce(jsonResponse(refused, 400));
      await api.restoreComposeVersion('foo', 3);
      const msgs = get(toasts).map((t) => t.message);
      expect(msgs).toEqual([expected, expected]);
    });

    it('env save toasts the reasons', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'refused', violations: ['a'] }, 409));
      const res = await api.putEnv('foo', [{ key: 'A', value: '1' }]);
      expect(res.error).toBe('refused:\n- a');
      expect(get(toasts)[0].message).toBe('refused:\n- a');
    });

    it('importApp and importAppPreview include violations', async () => {
      const file = new Blob(['zip']);
      fetchMock.mockResolvedValueOnce(jsonResponse(refused, 400));
      let res = await api.importAppPreview(file, { mode: 'new', slug: 'foo' });
      expect(res.error).toBe(expected);
      expect(res.violations).toEqual(refused.violations);
      fetchMock.mockResolvedValueOnce(jsonResponse(refused, 400));
      res = await api.importApp(file, { mode: 'new', slug: 'foo' });
      expect(res.error).toBe(expected);
      expect(res.data).toEqual(refused);
    });

    it('uploadRestore includes violations from a JSON error', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'refused', violations: ['a'] }, 400));
      const res = await api.uploadRestore('foo', new FormData());
      expect(res.error).toBe('refused:\n- a');
      expect(res.violations).toEqual(['a']);
    });

    it('uploadRestore keeps its fallback message for an empty body', async () => {
      fetchMock.mockResolvedValueOnce(textResponse('', 500));
      const res = await api.uploadRestore('foo', new FormData());
      expect(res.error).toBe('Upload failed');
    });
  });
});
