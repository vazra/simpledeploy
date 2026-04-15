import { getState } from './auth.js';

let sessionCookie = null;

export async function apiRequest(method, path, body) {
  const state = getState();
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (sessionCookie) {
    opts.headers['Cookie'] = sessionCookie;
  }
  if (body) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${state.baseURL}${path}`, opts);
  const setCookie = res.headers.get('set-cookie');
  if (setCookie && setCookie.includes('session=')) {
    sessionCookie = setCookie.split(';')[0];
  }
  const text = await res.text();
  let data;
  try { data = JSON.parse(text); } catch { data = text; }
  return { status: res.status, data, ok: res.ok };
}

export async function apiLogin(username, password) {
  return apiRequest('POST', '/api/auth/login', { username, password });
}

export async function waitForAppStatus(slug, status, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}`);
    if (res.ok && res.data.status === status) return res.data;
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`App ${slug} did not reach status "${status}" within ${timeoutMs}ms`);
}
