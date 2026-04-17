import { getState } from './auth.js';
import { execFileSync } from 'child_process';

// Node's fetch silently drops the `Host` header (forbidden per WHATWG).
// Use curl so we can force the Host header needed for Caddy routing.
export async function fetchViaProxy(hostHeader, path = '/', opts = {}) {
  const state = getState();
  const base = opts.proxyURL || state.proxyURL;
  const port = new URL(base).port;
  const args = ['-sS', '-o', '-', '-w', '\n__HTTP_CODE__%{http_code}'];
  args.push('--resolve', `${hostHeader}:${port}:127.0.0.1`);
  if (opts.insecure) args.push('-k');
  if (opts.method) args.push('-X', opts.method);
  if (opts.body) args.push('--data', typeof opts.body === 'string' ? opts.body : JSON.stringify(opts.body));
  if (opts.headers) {
    for (const [k, v] of Object.entries(opts.headers)) args.push('-H', `${k}: ${v}`);
  }
  const url = `http://${hostHeader}:${port}${path}`;
  args.push(url);
  let raw;
  let status = 0;
  try {
    raw = execFileSync('curl', args, { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] });
    const m = raw.match(/\n__HTTP_CODE__(\d+)$/);
    status = m ? Number(m[1]) : 0;
    raw = raw.replace(/\n__HTTP_CODE__\d+$/, '');
  } catch (e) {
    raw = '';
    status = 0;
  }
  return {
    status,
    ok: status >= 200 && status < 300,
    text: async () => raw,
    json: async () => { try { return JSON.parse(raw); } catch { return null; } },
  };
}

export function curlViaProxy(hostHeader, path = '/', opts = {}) {
  const state = getState();
  const base = opts.proxyURL || state.proxyURL;
  const args = ['-sS', '-o', '-', '-w', '\n__HTTP_CODE__%{http_code}', '--resolve', `${hostHeader}:${new URL(base).port}:127.0.0.1`];
  if (opts.insecure) args.push('-k');
  if (opts.method) args.push('-X', opts.method);
  if (opts.data) args.push('--data', opts.data);
  if (opts.headers) {
    for (const [k, v] of Object.entries(opts.headers)) args.push('-H', `${k}: ${v}`);
  }
  const url = `${base.replace('localhost', hostHeader).replace('127.0.0.1', hostHeader)}${path}`;
  args.push(url);
  try {
    const out = execFileSync('curl', args, { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] });
    const m = out.match(/\n__HTTP_CODE__(\d+)$/);
    return { status: m ? Number(m[1]) : 0, body: out.replace(/\n__HTTP_CODE__\d+$/, '') };
  } catch (e) {
    return { status: 0, body: '', error: e.message };
  }
}

export function curlHTTPS(host, port, path = '/', opts = {}) {
  const args = [
    '-sSk',
    '-o', '-',
    '-w', '\n__HTTP_CODE__%{http_code}',
    '--resolve', `${host}:${port}:127.0.0.1`,
    `https://${host}:${port}${path}`,
  ];
  if (opts.headers) {
    for (const [k, v] of Object.entries(opts.headers)) args.push('-H', `${k}: ${v}`);
  }
  try {
    const out = execFileSync('curl', args, { encoding: 'utf-8', stdio: ['ignore', 'pipe', 'pipe'] });
    const m = out.match(/\n__HTTP_CODE__(\d+)$/);
    return { status: m ? Number(m[1]) : 0, body: out.replace(/\n__HTTP_CODE__\d+$/, '') };
  } catch (e) {
    return { status: 0, body: '', error: e.message };
  }
}

export function openSSLGetCert(host, port) {
  try {
    const out = execFileSync(
      'sh',
      ['-c', `echo | openssl s_client -connect 127.0.0.1:${port} -servername ${host} 2>/dev/null | openssl x509 -noout -subject -issuer -dates`],
      { encoding: 'utf-8' },
    );
    const info = {};
    for (const line of out.split('\n')) {
      const [k, ...rest] = line.split('=');
      if (k && rest.length) info[k.trim()] = rest.join('=').trim();
    }
    return info;
  } catch (e) {
    return { error: e.message };
  }
}
