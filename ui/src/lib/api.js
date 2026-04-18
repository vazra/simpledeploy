import { toasts } from './stores/toast.js'

const BASE = '/api'

async function baseRequest(method, path, body, responseMode) {
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
      return { data: null, error, status: res.status }
    }
    let data
    if (responseMode === 'text') {
      data = await res.text()
    } else {
      const ct = res.headers.get('content-type')
      data = ct && ct.includes('application/json') ? await res.json() : null
    }
    return { data, error: null, status: res.status }
  } catch (err) {
    clearTimeout(timeout)
    const msg = err.name === 'AbortError' ? 'Request timed out' : err.message
    return { data: null, error: msg }
  }
}

function request(method, path, body = null) {
  return baseRequest(method, path, body, 'json')
}

function requestText(method, path, body = null) {
  return baseRequest(method, path, body, 'text')
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
  setup: (username, password, display_name, email) => request('POST', '/setup', { username, password, display_name, email }),
  health: healthCheck,

  // Apps
  listApps: () => request('GET', '/apps'),
  getApp: (slug) => request('GET', `/apps/${slug}`),
  removeApp: (slug) => requestWithToast('DELETE', `/apps/${slug}`, null, 'App removed'),
  deploy: (name, compose, force = false) => request('POST', '/apps/deploy', { name, compose, force }),
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
  deleteVersion: (slug, id) => requestWithToast('DELETE', `/apps/${slug}/versions/${id}`, null, 'Version deleted'),
  getDeployEvents: (slug) => request('GET', `/apps/${slug}/events`),
  updateDomain: (slug, domain) => requestWithToast('PUT', `/apps/${slug}/domain`, { domain }, 'Domain updated'),
  updateEndpoints: (slug, endpoints) => requestWithToast('PUT', `/apps/${slug}/endpoints`, endpoints, 'Endpoints updated'),
  uploadCert: (slug, domain, cert, key) => requestWithToast('PUT', `/apps/${slug}/certs/${encodeURIComponent(domain)}`, { cert, key }, 'Certificate uploaded'),
  deleteCert: (slug, domain) => requestWithToast('DELETE', `/apps/${slug}/certs/${encodeURIComponent(domain)}`, null, 'Certificate removed'),
  updateAccess: (slug, allow) => requestWithToast('PUT', `/apps/${slug}/access`, { allow }, 'IP allowlist updated'),

  // Metrics
  systemMetrics: (range) => request('GET', `/metrics/system?range=${range || '1h'}`),
  appMetrics: (slug, range) => request('GET', `/apps/${slug}/metrics?range=${range || '1h'}`),
  appRequests: (slug, range) => request('GET', `/apps/${slug}/requests?range=${range || '1h'}`),

  // Backup configs
  listBackupConfigs: (slug) => request('GET', `/apps/${slug}/backups/configs`),
  createBackupConfig: (slug, cfg) => requestWithToast('POST', `/apps/${slug}/backups/configs`, cfg, 'Backup config created'),
  updateBackupConfig: (id, cfg) => requestWithToast('PUT', `/backups/configs/${id}`, cfg, 'Backup config updated'),
  deleteBackupConfig: (id) => requestWithToast('DELETE', `/backups/configs/${id}`, null, 'Backup config deleted'),

  // Backup runs
  listBackupRuns: (slug) => request('GET', `/apps/${slug}/backups/runs`),
  triggerBackup: (slug) => requestWithToast('POST', `/apps/${slug}/backups/run`, null, 'Backup triggered'),
  triggerBackupConfig: (id) => requestWithToast('POST', `/backups/configs/${id}/run`, null, 'Backup triggered'),
  restore: (id) => requestWithToast('POST', `/backups/restore/${id}`, null, 'Restore started'),
  downloadBackupUrl: (id) => `${BASE}/backups/runs/${id}/download`,
  uploadRestore: async (slug, formData) => {
    try {
      const res = await fetch(`${BASE}/apps/${slug}/backups/upload-restore`, {
        method: 'POST',
        body: formData,
        credentials: 'include',
      });
      if (!res.ok) {
        const text = await res.text();
        return { data: null, error: text || 'Upload failed' };
      }
      return { data: true, error: null };
    } catch (err) {
      return { data: null, error: err.message };
    }
  },

  // Backup dashboard & detection
  backupSummary: () => request('GET', '/backups/summary'),
  detectStrategies: (slug) => request('GET', `/apps/${slug}/backups/detect`),
  testS3: (cfg) => request('POST', '/backups/test-s3', cfg),

  // Compose versions
  updateComposeVersion: (slug, id, data) => requestWithToast('PUT', `/apps/${slug}/versions/${id}`, data, 'Version updated'),
  downloadComposeVersionUrl: (slug, id) => `${BASE}/apps/${slug}/versions/${id}/download`,
  restoreComposeVersion: (slug, id) => requestWithToast('POST', `/apps/${slug}/versions/${id}/restore`, null, 'Restoring version'),

  // Webhooks
  listWebhooks: () => request('GET', '/webhooks'),
  createWebhook: (w) => requestWithToast('POST', '/webhooks', w, 'Webhook created'),
  updateWebhook: (id, w) => requestWithToast('PUT', `/webhooks/${id}`, w, 'Webhook updated'),
  deleteWebhook: (id) => requestWithToast('DELETE', `/webhooks/${id}`, null, 'Webhook deleted'),
  testWebhook: (data) => requestWithToast('POST', '/webhooks/test', data, 'Test sent successfully'),

  // Alerts
  listAlertRules: () => request('GET', '/alerts/rules'),
  createAlertRule: (r) => requestWithToast('POST', '/alerts/rules', r, 'Alert rule created'),
  updateAlertRule: (id, r) => requestWithToast('PUT', `/alerts/rules/${id}`, r, 'Alert rule updated'),
  deleteAlertRule: (id) => requestWithToast('DELETE', `/alerts/rules/${id}`, null, 'Alert rule deleted'),
  alertHistory: () => request('GET', '/alerts/history'),
  clearAlertHistory: (mode) => requestWithToast('DELETE', `/alerts/history?mode=${mode}`, null, 'Alert history cleared'),

  // Users
  listUsers: () => request('GET', '/users'),
  createUser: (u) => requestWithToast('POST', '/users', u, 'User created'),
  updateUser: (id, u) => requestWithToast('PUT', `/users/${id}`, u, 'User updated'),
  deleteUser: (id) => requestWithToast('DELETE', `/users/${id}`, null, 'User deleted'),
  listAPIKeys: () => request('GET', '/apikeys'),
  createAPIKey: (name) => requestWithToast('POST', '/apikeys', { name }, 'API key created'),
  deleteAPIKey: (id) => requestWithToast('DELETE', `/apikeys/${id}`, null, 'API key revoked'),

  // Profile
  getProfile: () => request('GET', '/me'),
  updateProfile: (data) => requestWithToast('PUT', '/me', data, 'Profile updated'),
  changePassword: (data) => requestWithToast('PUT', '/me/password', data, 'Password changed'),

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
  systemLogsWs: () => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return new WebSocket(`${proto}//${window.location.host}/api/system/process-logs/stream`)
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
  systemLogs: (limit = 500) => request('GET', `/system/process-logs?limit=${limit}`),
  systemBackupDownload: (compact = false) => {
    const url = `/api/system/backup/download?compact=${compact}`
    return fetch(url, { method: 'POST', credentials: 'include' }).then(res => {
      if (!res.ok) return res.text().then(t => ({ error: t }))
      return res.blob().then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = res.headers.get('content-disposition')?.match(/filename="?([^"]+)"?/)?.[1] || 'backup.db'
        a.click()
        URL.revokeObjectURL(a.href)
        return { data: true, error: null }
      })
    }).catch(err => ({ data: null, error: err.message }))
  },
  systemBackupConfig: () => request('GET', '/system/backup/config'),
  systemSetBackupConfig: (cfg) => request('POST', '/system/backup/config', cfg),
  systemBackupRuns: () => request('GET', '/system/backup/runs'),

  // Public host (used by template quick-test mode)
  getPublicHost: () => request('GET', '/system/public-host'),
  setPublicHost: (host) => requestWithToast('PUT', '/system/public-host', { public_host: host }, 'Default host saved'),
}
