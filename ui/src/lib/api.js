const BASE = '/api'

async function request(method, path, body = null) {
  const opts = {
    method,
    headers: {},
    credentials: 'include',
  }
  if (body) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(BASE + path, opts)
  if (res.status === 401) {
    if (!window.location.hash.includes('login')) {
      window.location.hash = '#/login'
    }
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `HTTP ${res.status}`)
  }
  const ct = res.headers.get('content-type')
  if (ct && ct.includes('application/json')) return res.json()
  return null
}

export const api = {
  login: (username, password) => request('POST', '/auth/login', { username, password }),
  logout: () => request('POST', '/auth/logout'),
  setup: (username, password) => request('POST', '/setup', { username, password }),
  health: () => request('GET', '/health'),
  listApps: () => request('GET', '/apps'),
  getApp: (slug) => request('GET', `/apps/${slug}`),
  removeApp: (slug) => request('DELETE', `/apps/${slug}`),
  systemMetrics: (from, to) => request('GET', `/metrics/system?from=${from}&to=${to}`),
  appMetrics: (slug, from, to) => request('GET', `/apps/${slug}/metrics?from=${from}&to=${to}`),
  appRequests: (slug, from, to) => request('GET', `/apps/${slug}/requests?from=${from}&to=${to}`),
  listBackupConfigs: (slug) => request('GET', `/apps/${slug}/backups/configs`),
  createBackupConfig: (slug, cfg) => request('POST', `/apps/${slug}/backups/configs`, cfg),
  deleteBackupConfig: (id) => request('DELETE', `/backups/configs/${id}`),
  listBackupRuns: (slug) => request('GET', `/apps/${slug}/backups/runs`),
  triggerBackup: (slug) => request('POST', `/apps/${slug}/backups/run`),
  restore: (id) => request('POST', `/backups/restore/${id}`),
  listWebhooks: () => request('GET', '/webhooks'),
  createWebhook: (w) => request('POST', '/webhooks', w),
  deleteWebhook: (id) => request('DELETE', `/webhooks/${id}`),
  listAlertRules: () => request('GET', '/alerts/rules'),
  createAlertRule: (r) => request('POST', '/alerts/rules', r),
  deleteAlertRule: (id) => request('DELETE', `/alerts/rules/${id}`),
  alertHistory: () => request('GET', '/alerts/history'),
  listUsers: () => request('GET', '/users'),
  createUser: (u) => request('POST', '/users', u),
  deleteUser: (id) => request('DELETE', `/users/${id}`),
  listAPIKeys: () => request('GET', '/apikeys'),
  createAPIKey: (name) => request('POST', '/apikeys', { name }),
  deleteAPIKey: (id) => request('DELETE', `/apikeys/${id}`),
}
