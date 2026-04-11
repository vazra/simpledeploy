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
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 30000)
  opts.signal = controller.signal
  try {
    const res = await fetch(BASE + path, opts)
    clearTimeout(timeout)
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
    clearTimeout(timeout)
    const msg = err.name === 'AbortError' ? 'Request timed out' : err.message
    return { data: null, error: msg }
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
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 30000)
  opts.signal = controller.signal
  try {
    const res = await fetch(BASE + path, opts)
    clearTimeout(timeout)
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
    clearTimeout(timeout)
    const msg = err.name === 'AbortError' ? 'Request timed out' : err.message
    return { data: null, error: msg }
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

async function healthCheck() {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 5000)
  try {
    const res = await fetch(BASE + '/health', { signal: controller.signal })
    clearTimeout(timeout)
    return { data: res.ok, error: res.ok ? null : `HTTP ${res.status}` }
  } catch (err) {
    clearTimeout(timeout)
    return { data: null, error: err.name === 'AbortError' ? 'Request timed out' : err.message }
  }
}

export const api = {
  // Auth (no toast on success for login)
  login: (username, password) => request('POST', '/auth/login', { username, password }),
  logout: () => request('POST', '/auth/logout'),
  setupStatus: () => request('GET', '/setup/status'),
  setup: (username, password) => request('POST', '/setup', { username, password }),
  health: healthCheck,

  // Apps
  listApps: () => request('GET', '/apps'),
  getApp: (slug) => request('GET', `/apps/${slug}`),
  removeApp: (slug) => requestWithToast('DELETE', `/apps/${slug}`, null, 'App removed'),
  deploy: (name, compose) => requestWithToast('POST', '/apps/deploy', { name, compose }, 'App deployed'),
  getCompose: (slug) => requestText('GET', `/apps/${slug}/compose`),
  validateCompose: (compose) => request('POST', '/apps/validate-compose', { compose }),
  restartApp: (slug) => request('POST', `/apps/${slug}/restart`),
  stopApp: (slug) => requestWithToast('POST', `/apps/${slug}/stop`, null, 'App stopped'),
  startApp: (slug) => requestWithToast('POST', `/apps/${slug}/start`, null, 'App started'),
  pullApp: (slug) => request('POST', `/apps/${slug}/pull`),
  cancelDeploy: (slug) => requestWithToast('POST', `/apps/${slug}/cancel`, null, 'Deploy cancelled'),
  scaleApp: (slug, scales) => requestWithToast('POST', `/apps/${slug}/scale`, { scales }, 'App scaled'),
  getAppServices: (slug) => request('GET', `/apps/${slug}/services`),
  getEnv: (slug) => request('GET', `/apps/${slug}/env`),
  putEnv: (slug, vars) => requestWithToast('PUT', `/apps/${slug}/env`, vars, 'Environment saved'),
  getComposeVersions: (slug) => request('GET', `/apps/${slug}/versions`),
  rollbackApp: (slug, versionId) => requestWithToast('POST', `/apps/${slug}/rollback`, { version_id: versionId }, 'Rolled back'),
  getDeployEvents: (slug) => request('GET', `/apps/${slug}/events`),
  updateDomain: (slug, domain) => requestWithToast('PUT', `/apps/${slug}/domain`, { domain }, 'Domain updated'),
  updateAccess: (slug, allow) => requestWithToast('PUT', `/apps/${slug}/access`, { allow }, 'IP allowlist updated'),

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

  // Docker
  dockerInfo: () => request('GET', '/docker/info'),
  dockerDiskUsage: () => request('GET', '/docker/disk-usage'),
  dockerImages: () => request('GET', '/docker/images'),
  dockerRemoveImage: (id) => requestWithToast('DELETE', `/docker/images/${encodeURIComponent(id)}`, null, 'Image removed'),
  dockerNetworks: () => request('GET', '/docker/networks'),
  dockerVolumes: () => request('GET', '/docker/volumes'),
  dockerRemoveNetwork: (id) => requestWithToast('DELETE', `/docker/networks/${encodeURIComponent(id)}`, null, 'Network removed'),
  dockerRemoveVolume: (name) => requestWithToast('DELETE', `/docker/volumes/${encodeURIComponent(name)}`, null, 'Volume removed'),
  dockerPruneContainers: () => request('POST', '/docker/prune/containers'),
  dockerPruneImages: () => request('POST', '/docker/prune/images'),
  dockerPruneVolumes: () => request('POST', '/docker/prune/volumes'),
  dockerPruneBuildCache: () => request('POST', '/docker/prune/build-cache'),
  dockerPruneAll: () => request('POST', '/docker/prune/all'),

  // WebSocket
  deployLogsWs: (slug) => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return new WebSocket(`${proto}//${window.location.host}/api/apps/${slug}/deploy-logs`)
  },

  // System
  systemInfo: () => request('GET', '/system/info'),
  systemStorageBreakdown: () => request('GET', '/system/storage-breakdown'),
  systemPruneMetrics: (days, tier) => request('POST', '/system/prune/metrics', { days, tier }),
  systemPruneRequestStats: (days, tier) => request('POST', '/system/prune/request-stats', { days, tier }),
  systemVacuum: () => request('POST', '/system/vacuum'),
  systemAuditLog: (limit = 200) => request('GET', `/system/audit-log?limit=${limit}`),
  systemClearAuditLog: () => request('DELETE', '/system/audit-log'),
  systemAuditConfig: () => request('GET', '/system/audit-config'),
  systemUpdateAuditConfig: (maxSize) => request('PUT', '/system/audit-config', { max_size: maxSize }),
}
