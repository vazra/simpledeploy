import { toasts } from './stores/toast.js'

const BASE = '/api'

async function requestText(method, path, body = null) {
  const opts = {
    method,
    headers: {},
    credentials: 'include',
  }
  if (body) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  try {
    const res = await fetch(BASE + path, opts)
    if (res.status === 401) {
      if (!window.location.hash.includes('login')) {
        window.location.hash = '#/login'
      }
      return { data: null, error: 'Unauthorized' }
    }
    if (!res.ok) {
      const text = await res.text()
      return { data: null, error: text || `HTTP ${res.status}` }
    }
    const data = await res.text()
    return { data, error: null }
  } catch (err) {
    return { data: null, error: err.message }
  }
}

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
  try {
    const res = await fetch(BASE + path, opts)
    if (res.status === 401) {
      if (!window.location.hash.includes('login')) {
        window.location.hash = '#/login'
      }
      return { data: null, error: 'Unauthorized' }
    }
    if (!res.ok) {
      const text = await res.text()
      const error = text || `HTTP ${res.status}`
      return { data: null, error }
    }
    const ct = res.headers.get('content-type')
    const data = ct && ct.includes('application/json') ? await res.json() : null
    return { data, error: null }
  } catch (err) {
    return { data: null, error: err.message }
  }
}

async function requestWithToast(method, path, body, successMsg) {
  const result = await request(method, path, body)
  if (result.error) {
    toasts.error(result.error)
  } else if (successMsg) {
    toasts.success(successMsg)
  }
  return result
}

export const api = {
  // Auth (no toast on success for login)
  login: (username, password) => request('POST', '/auth/login', { username, password }),
  logout: () => request('POST', '/auth/logout'),
  setup: (username, password) => request('POST', '/setup', { username, password }),
  health: () => request('GET', '/health'),

  // Apps
  listApps: () => request('GET', '/apps'),
  getApp: (slug) => request('GET', `/apps/${slug}`),
  removeApp: (slug) => requestWithToast('DELETE', `/apps/${slug}`, null, 'App removed'),
  deploy: (name, compose) => requestWithToast('POST', '/apps/deploy', { name, compose }, 'App deployed'),
  getCompose: (slug) => requestText('GET', `/apps/${slug}/compose`),
  validateCompose: (compose) => request('POST', '/apps/validate-compose', { compose }),
  restartApp: (slug) => requestWithToast('POST', `/apps/${slug}/restart`, null, 'App restarted'),
  stopApp: (slug) => requestWithToast('POST', `/apps/${slug}/stop`, null, 'App stopped'),
  startApp: (slug) => requestWithToast('POST', `/apps/${slug}/start`, null, 'App started'),
  pullApp: (slug) => requestWithToast('POST', `/apps/${slug}/pull`, null, 'Images pulled & redeployed'),
  scaleApp: (slug, scales) => requestWithToast('POST', `/apps/${slug}/scale`, { scales }, 'App scaled'),
  getAppServices: (slug) => request('GET', `/apps/${slug}/services`),
  getComposeVersions: (slug) => request('GET', `/apps/${slug}/versions`),
  rollbackApp: (slug, versionId) => requestWithToast('POST', `/apps/${slug}/rollback`, { version_id: versionId }, 'Rolled back'),
  getDeployEvents: (slug) => request('GET', `/apps/${slug}/events`),

  // Metrics
  systemMetrics: (from, to) => request('GET', `/metrics/system?from=${from}&to=${to}`),
  appMetrics: (slug, from, to) => request('GET', `/apps/${slug}/metrics?from=${from}&to=${to}`),
  appRequests: (slug, from, to) => request('GET', `/apps/${slug}/requests?from=${from}&to=${to}`),

  // Backups
  listBackupConfigs: (slug) => request('GET', `/apps/${slug}/backups/configs`),
  createBackupConfig: (slug, cfg) => requestWithToast('POST', `/apps/${slug}/backups/configs`, cfg, 'Backup config created'),
  deleteBackupConfig: (id) => requestWithToast('DELETE', `/backups/configs/${id}`, null, 'Backup config deleted'),
  listBackupRuns: (slug) => request('GET', `/apps/${slug}/backups/runs`),
  triggerBackup: (slug) => requestWithToast('POST', `/apps/${slug}/backups/run`, null, 'Backup triggered'),
  restore: (id) => requestWithToast('POST', `/backups/restore/${id}`, null, 'Restore started'),

  // Webhooks
  listWebhooks: () => request('GET', '/webhooks'),
  createWebhook: (w) => requestWithToast('POST', '/webhooks', w, 'Webhook created'),
  deleteWebhook: (id) => requestWithToast('DELETE', `/webhooks/${id}`, null, 'Webhook deleted'),

  // Alerts
  listAlertRules: () => request('GET', '/alerts/rules'),
  createAlertRule: (r) => requestWithToast('POST', '/alerts/rules', r, 'Alert rule created'),
  deleteAlertRule: (id) => requestWithToast('DELETE', `/alerts/rules/${id}`, null, 'Alert rule deleted'),
  alertHistory: () => request('GET', '/alerts/history'),

  // Users
  listUsers: () => request('GET', '/users'),
  createUser: (u) => requestWithToast('POST', '/users', u, 'User created'),
  deleteUser: (id) => requestWithToast('DELETE', `/users/${id}`, null, 'User deleted'),
  listAPIKeys: () => request('GET', '/apikeys'),
  createAPIKey: (name) => requestWithToast('POST', '/apikeys', { name }, 'API key created'),
  deleteAPIKey: (id) => requestWithToast('DELETE', `/apikeys/${id}`, null, 'API key revoked'),

  // Registries
  listRegistries: () => request('GET', '/registries'),
  createRegistry: (r) => requestWithToast('POST', '/registries', r, 'Registry added'),
  updateRegistry: (id, r) => requestWithToast('PUT', `/registries/${id}`, r, 'Registry updated'),
  deleteRegistry: (id) => requestWithToast('DELETE', `/registries/${id}`, null, 'Registry removed'),
}
