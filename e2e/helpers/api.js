import { getState } from './auth.js';

let sessionCookie = null;

export function getSessionCookie() {
  return sessionCookie;
}

export async function apiDownload(path) {
  const state = getState();
  const opts = { method: 'GET', headers: {} };
  if (sessionCookie) opts.headers['Cookie'] = sessionCookie;
  const res = await fetch(`${state.baseURL}${path}`, opts);
  if (!res.ok) {
    const text = await res.text();
    return { ok: false, status: res.status, body: text, buffer: null };
  }
  const buf = Buffer.from(await res.arrayBuffer());
  return { ok: true, status: res.status, buffer: buf };
}

export async function apiUploadMultipart(path, fields, fileField, fileName, fileBuffer) {
  const state = getState();
  const form = new FormData();
  for (const [k, v] of Object.entries(fields || {})) {
    form.append(k, v);
  }
  const blob = new Blob([fileBuffer]);
  form.append(fileField, blob, fileName);
  const opts = { method: 'POST', body: form, headers: {} };
  if (sessionCookie) opts.headers['Cookie'] = sessionCookie;
  const res = await fetch(`${state.baseURL}${path}`, opts);
  const text = await res.text();
  let data; try { data = JSON.parse(text); } catch { data = text; }
  return { ok: res.ok, status: res.status, data };
}

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

export async function apiRequestWithKey(method, path, body, apiKey) {
  const state = getState();
  const opts = {
    method,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${apiKey}`,
    },
  };
  if (body) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${state.baseURL}${path}`, opts);
  const text = await res.text();
  let data;
  try { data = JSON.parse(text); } catch { data = text; }
  return { status: res.status, data, ok: res.ok };
}

export async function waitForAppStatus(slug, status, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await apiRequest('GET', `/api/apps/${slug}`);
    // store.App has no JSON tags; fields serialize as PascalCase.
    const cur = res.data && (res.data.Status || res.data.status);
    if (res.ok && cur === status) return res.data;
    await new Promise((r) => setTimeout(r, 1_000));
  }
  throw new Error(`App ${slug} did not reach status "${status}" within ${timeoutMs}ms`);
}

// apiRequestAt makes a request against an arbitrary baseURL (for isolated test servers).
// Unlike apiRequest it does not share module-level session state. Pass a sessionCookie
// (captured from a prior login's set-cookie) to authenticate.
// Returns { status, data, ok, setCookie }.
export async function apiRequestAt(baseURL, method, path, body, sessionCookie) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (sessionCookie) {
    opts.headers['Cookie'] = sessionCookie;
  }
  if (body !== undefined && body !== null) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${baseURL}${path}`, opts);
  const setCookieHdr = res.headers.get('set-cookie');
  let setCookie = null;
  if (setCookieHdr && setCookieHdr.includes('session=')) {
    setCookie = setCookieHdr.split(';')[0];
  }
  const text = await res.text();
  let data;
  try { data = JSON.parse(text); } catch { data = text; }
  return { status: res.status, data, ok: res.ok, setCookie };
}
