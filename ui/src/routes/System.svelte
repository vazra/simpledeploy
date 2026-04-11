<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'
  import { toasts } from '../lib/stores/toast.js'

  let activeTab = $state('overview')
  let loading = $state(false)
  let info = $state(null)
  let breakdown = $state(null)
  let breakdownLoading = $state(false)

  let metricsDays = $state(30)
  let metricsTier = $state('raw')
  let reqStatsDays = $state(30)
  let reqStatsTier = $state('raw')
  let pruningMetrics = $state(false)
  let pruningReqStats = $state(false)
  let vacuuming = $state(false)

  let auditLogs = $state([])
  let auditLoading = $state(false)
  let clearing = $state(false)
  let auditMaxSize = $state(500)
  let savingConfig = $state(false)

  // Logs tab
  let processLogs = $state([])
  let logsLoading = $state(false)
  let logsWs = $state(null)
  let logsStreaming = $state(false)
  let logsAutoScroll = $state(true)

  // DB Backup
  let backupConfig = $state({ schedule: '', destination: '', retention: 7, compact: false, enabled: false })
  let backupRuns = $state([])
  let backupLoading = $state(false)
  let downloading = $state(false)
  let downloadCompact = $state(false)
  let savingBackupConfig = $state(false)

  const tiers = ['raw', '1m', '5m', '1h']
  const tierLabels = { raw: 'Raw', '1m': '1 min', '5m': '5 min', '1h': '1 hour' }

  function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  onMount(() => {
    Promise.all([loadInfo(), loadBreakdown()])
  })

  async function loadInfo() {
    loading = true
    const res = await api.systemInfo()
    if (res.data) info = res.data
    loading = false
  }

  async function loadBreakdown() {
    breakdownLoading = true
    const res = await api.systemStorageBreakdown()
    if (res.error) {
      toasts.error('Breakdown: ' + res.error)
    } else if (res.data) {
      breakdown = res.data
    }
    breakdownLoading = false
  }

  async function pruneMetrics() {
    pruningMetrics = true
    const res = await api.systemPruneMetrics(metricsDays, metricsTier)
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success(res.data?.message || `Metrics[${metricsTier}] pruned`)
      loadBreakdown()
    }
    pruningMetrics = false
  }

  async function pruneReqStats() {
    pruningReqStats = true
    const res = await api.systemPruneRequestStats(reqStatsDays, reqStatsTier)
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success(res.data?.message || `Request stats[${reqStatsTier}] pruned`)
      loadBreakdown()
    }
    pruningReqStats = false
  }

  async function vacuum() {
    vacuuming = true
    const res = await api.systemVacuum()
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success('VACUUM completed')
      loadInfo()
    }
    vacuuming = false
  }

  async function loadAuditLogs() {
    auditLoading = true
    const [logsRes, cfgRes] = await Promise.all([
      api.systemAuditLog(500),
      api.systemAuditConfig(),
    ])
    if (logsRes.data) auditLogs = logsRes.data
    if (cfgRes.data) auditMaxSize = cfgRes.data.max_size
    auditLoading = false
  }

  async function clearAuditLogs() {
    clearing = true
    const res = await api.systemClearAuditLog()
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success('Audit log cleared')
      await loadAuditLogs()
    }
    clearing = false
  }

  async function saveAuditConfig() {
    savingConfig = true
    const res = await api.systemUpdateAuditConfig(auditMaxSize)
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success(`Buffer resized to ${auditMaxSize} events`)
    }
    savingConfig = false
  }

  async function loadLogs() {
    logsLoading = true
    const res = await api.systemLogs(1000)
    if (res.data) processLogs = res.data
    if (res.error) toasts.error('Logs: ' + res.error)
    logsLoading = false
  }

  function startLogsStream() {
    if (logsWs) logsWs.close()
    const ws = api.systemLogsWs()
    ws.onmessage = (e) => {
      const entry = JSON.parse(e.data)
      processLogs = [...processLogs, entry]
      // Keep buffer bounded
      if (processLogs.length > 2000) processLogs = processLogs.slice(-1000)
      if (logsAutoScroll) {
        requestAnimationFrame(() => {
          const el = document.getElementById('logs-container')
          if (el) el.scrollTop = el.scrollHeight
        })
      }
    }
    ws.onclose = () => { logsStreaming = false; logsWs = null }
    ws.onerror = () => { logsStreaming = false }
    logsWs = ws
    logsStreaming = true
  }

  function stopLogsStream() {
    if (logsWs) { logsWs.close(); logsWs = null }
    logsStreaming = false
  }

  async function downloadBackup() {
    downloading = true
    const res = await api.systemBackupDownload(downloadCompact)
    if (res?.error) toasts.error(res.error)
    else toasts.success('Backup downloaded')
    downloading = false
  }

  async function loadBackupConfig() {
    backupLoading = true
    const [cfgRes, runsRes] = await Promise.all([
      api.systemBackupConfig(),
      api.systemBackupRuns(),
    ])
    if (cfgRes.data) {
      backupConfig = {
        schedule: cfgRes.data.schedule || '',
        destination: cfgRes.data.destination || '',
        retention: parseInt(cfgRes.data.retention) || 7,
        compact: cfgRes.data.compact === 'true',
        enabled: cfgRes.data.enabled === 'true',
      }
    }
    if (runsRes.data) backupRuns = runsRes.data
    backupLoading = false
  }

  async function saveBackupConfig() {
    savingBackupConfig = true
    const res = await api.systemSetBackupConfig(backupConfig)
    if (res.error) toasts.error(res.error)
    else toasts.success('Backup config saved')
    savingBackupConfig = false
  }

  function switchTab(tab) {
    activeTab = tab
    if (tab === 'audit' && auditLogs.length === 0) loadAuditLogs()
    if (tab === 'logs' && processLogs.length === 0) loadLogs()
    if (tab === 'maintenance' && backupRuns.length === 0) loadBackupConfig()
    if (tab !== 'logs') stopLogsStream()
  }

  const eventTypeColors = {
    login: 'text-emerald-400',
    login_failed: 'text-red-400',
    user_created: 'text-blue-400',
    user_deleted: 'text-orange-400',
    apikey_created: 'text-blue-400',
    apikey_deleted: 'text-orange-400',
    deploy: 'text-violet-400',
    audit_cleared: 'text-text-secondary',
  }

  function formatTime(ts) {
    if (!ts) return '-'
    const d = new Date(ts)
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const rowCountLabels = {
    apps: 'Apps',
    users: 'Users',
    metrics: 'Metrics',
    request_stats: 'Request Stats',
    alert_rules: 'Alert Rules',
    backup_runs: 'Backup Runs',
  }

  function totalRows(tierStats) {
    return (tierStats || []).reduce((s, t) => s + t.count, 0)
  }

  function tierBarWidth(count, tierStats) {
    const total = totalRows(tierStats)
    return total > 0 ? (count / total) * 100 : 0
  }
</script>

<Layout>
  <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">System</h1>
  </div>

  <div class="flex overflow-x-auto gap-1 mb-6 border-b border-border/50">
    {#each [['overview', 'Overview'], ['maintenance', 'Maintenance'], ['audit', 'Audit Log'], ['logs', 'Logs']] as [key, label]}
      <button
        onclick={() => switchTab(key)}
        class="px-4 py-3 text-sm font-medium border-b-2 whitespace-nowrap shrink-0 transition-colors {activeTab === key ? 'border-accent text-accent' : 'border-transparent text-text-muted hover:text-text-primary'}"
      >{label}</button>
    {/each}
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={3} />
    </div>
  {:else if activeTab === 'overview'}
    {#if info}
      <!-- SimpleDeploy -->
      <h2 class="text-base font-medium text-text-primary mb-4">SimpleDeploy</h2>
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-8">
        <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
          <div>
            <div class="text-xs font-medium text-text-secondary">Version</div>
            <div class="text-sm font-semibold text-text-primary">{info.simpledeploy?.version || '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary">Commit</div>
            <div class="text-sm font-semibold text-text-primary font-mono">{(info.simpledeploy?.commit || '').slice(0, 7) || '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary">Build Date</div>
            <div class="text-sm font-semibold text-text-primary">{info.simpledeploy?.build_date || '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary">Uptime</div>
            <div class="text-sm font-semibold text-text-primary">{info.simpledeploy?.uptime || '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary">Go Version</div>
            <div class="text-sm font-semibold text-text-primary">{info.simpledeploy?.go_version || '-'}</div>
          </div>
        </div>
      </div>

      <!-- System Resources -->
      <h2 class="text-base font-medium text-text-primary mb-4">System Resources</h2>
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-8">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-6">
          <div>
            <div class="text-xs font-medium text-text-secondary mb-1">CPU Cores</div>
            <div class="text-xl font-bold text-text-primary">{info.resources?.cpu_count ?? '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary mb-1">RAM</div>
            {#if info.resources?.ram_total}
              <div class="flex items-center gap-2 mb-1">
                <div class="flex-1 bg-surface-3/30 rounded-full h-1.5 overflow-hidden">
                  <div
                    class="h-1.5 rounded-full transition-all {((info.resources.ram_used / info.resources.ram_total) * 100) > 85 ? 'bg-red-500' : ((info.resources.ram_used / info.resources.ram_total) * 100) > 70 ? 'bg-yellow-500' : 'bg-accent'}"
                    style="width: {Math.min((info.resources.ram_used / info.resources.ram_total) * 100, 100)}%"
                  ></div>
                </div>
                <span class="text-xs font-semibold text-text-primary whitespace-nowrap">{((info.resources.ram_used / info.resources.ram_total) * 100).toFixed(1)}%</span>
              </div>
              <div class="text-sm font-semibold text-text-primary">{formatBytes(info.resources.ram_used)} used</div>
              <div class="text-xs text-text-secondary">{formatBytes(info.resources.ram_avail)} free / {formatBytes(info.resources.ram_total)} total</div>
            {:else}
              <div class="text-sm text-text-secondary">Unavailable on this platform</div>
            {/if}
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary mb-2">Disk</div>
            <div class="flex items-center gap-2 mb-1">
              <div class="flex-1 bg-surface-3/30 rounded-full h-1.5 overflow-hidden">
                <div
                  class="h-1.5 rounded-full transition-all {(info.resources?.disk_used_pct || 0) > 85 ? 'bg-red-500' : (info.resources?.disk_used_pct || 0) > 70 ? 'bg-yellow-500' : 'bg-accent'}"
                  style="width: {Math.min(info.resources?.disk_used_pct || 0, 100)}%"
                ></div>
              </div>
              <span class="text-xs font-semibold text-text-primary whitespace-nowrap">{(info.resources?.disk_used_pct || 0).toFixed(1)}%</span>
            </div>
            <div class="text-sm font-semibold text-text-primary">{formatBytes(info.resources?.disk_used || 0)} used</div>
            <div class="text-xs text-text-secondary">{formatBytes(info.resources?.disk_avail || 0)} free / {formatBytes(info.resources?.disk_total || 0)} total</div>
          </div>
        </div>
      </div>

      <!-- Database -->
      <h2 class="text-base font-medium text-text-primary mb-4">Database</h2>
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-8">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-4">
          <div>
            <div class="text-xs font-medium text-text-secondary">Path</div>
            <div class="text-sm font-semibold text-text-primary font-mono truncate">{info.database?.path || '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary">File Size</div>
            <div class="text-sm font-semibold text-text-primary">{formatBytes(info.database?.size_bytes || 0)}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary">Migration Version</div>
            <div class="text-sm font-semibold text-text-primary">{info.database?.migration_version ?? '-'}</div>
          </div>
        </div>
        {#if info.database?.row_counts}
          <div class="border-t border-border pt-4">
            <div class="text-xs font-medium text-text-secondary mb-2">Row Counts</div>
            <table class="w-full text-sm">
              <tbody class="divide-y divide-border/30">
                {#each Object.entries(rowCountLabels) as [key, label]}
                  <tr class="hover:bg-surface-hover">
                    <td class="py-3 px-4 text-text-secondary text-xs">{label}</td>
                    <td class="py-3 px-4 text-right font-semibold text-text-primary text-xs">{(info.database.row_counts[key] ?? 0).toLocaleString()}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

      <!-- Storage Breakdown by Tier -->
      <h2 class="text-base font-medium text-text-primary mb-4">Metrics Storage Breakdown</h2>
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-8">
        {#if breakdownLoading}
          <Skeleton type="text" count={4} />
        {:else if breakdown}
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
            <!-- Metrics tiers -->
            <div>
              <div class="text-xs font-medium text-text-secondary mb-3">
                Metrics
                <span class="ml-1 text-text-primary font-semibold">{totalRows(breakdown.metrics).toLocaleString()} rows</span>
              </div>
              <div class="space-y-2">
                {#each tiers as tier}
                  {@const stat = breakdown.metrics?.find(s => s.tier === tier)}
                  {@const count = stat?.count ?? 0}
                  {@const pct = tierBarWidth(count, breakdown.metrics)}
                  <div>
                    <div class="flex justify-between text-xs mb-1">
                      <span class="text-text-secondary">{tierLabels[tier]}</span>
                      <span class="font-semibold text-text-primary">{count.toLocaleString()}</span>
                    </div>
                    <div class="bg-surface-3/30 rounded-full h-1 overflow-hidden">
                      <div class="h-1 rounded-full bg-accent transition-all" style="width: {pct}%"></div>
                    </div>
                  </div>
                {/each}
              </div>
            </div>

            <!-- Request Stats tiers -->
            <div>
              <div class="text-xs font-medium text-text-secondary mb-3">
                Request Stats
                <span class="ml-1 text-text-primary font-semibold">{totalRows(breakdown.request_stats).toLocaleString()} rows</span>
              </div>
              <div class="space-y-2">
                {#each tiers as tier}
                  {@const stat = breakdown.request_stats?.find(s => s.tier === tier)}
                  {@const count = stat?.count ?? 0}
                  {@const pct = tierBarWidth(count, breakdown.request_stats)}
                  <div>
                    <div class="flex justify-between text-xs mb-1">
                      <span class="text-text-secondary">{tierLabels[tier]}</span>
                      <span class="font-semibold text-text-primary">{count.toLocaleString()}</span>
                    </div>
                    <div class="bg-surface-3/30 rounded-full h-1 overflow-hidden">
                      <div class="h-1 rounded-full bg-accent transition-all" style="width: {pct}%"></div>
                    </div>
                  </div>
                {/each}
              </div>
            </div>
          </div>
        {:else}
          <p class="text-xs text-text-secondary">Failed to load breakdown.</p>
        {/if}
      </div>

      <!-- Apps -->
      <h2 class="text-base font-medium text-text-primary mb-4">Apps</h2>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
          <div class="text-xs font-medium text-text-secondary mb-1">Total</div>
          <div class="text-2xl font-semibold text-text-primary">{info.apps?.total ?? 0}</div>
        </div>
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
          <div class="text-xs font-medium text-text-secondary mb-1">Running</div>
          <div class="text-2xl font-semibold text-green-500">{info.apps?.running ?? 0}</div>
        </div>
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
          <div class="text-xs font-medium text-text-secondary mb-1">Stopped</div>
          <div class="text-2xl font-semibold text-text-secondary">{info.apps?.stopped ?? 0}</div>
        </div>
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
          <div class="text-xs font-medium text-text-secondary mb-1">Error</div>
          <div class="text-2xl font-semibold text-red-500">{info.apps?.error ?? 0}</div>
        </div>
      </div>
    {:else}
      <p class="text-sm text-text-muted">Failed to load system info.</p>
    {/if}

  {:else if activeTab === 'maintenance'}
    <div class="space-y-4">
      <!-- Prune Metrics -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Prune Metrics</h3>
        <p class="text-xs text-text-secondary mb-4">Delete metrics data for a specific resolution tier older than N days.</p>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex items-center gap-2">
            <span class="text-xs text-text-secondary">Tier</span>
            <select
              bind:value={metricsTier}
              class="px-2 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
            >
              {#each tiers as t}
                <option value={t}>{tierLabels[t]}</option>
              {/each}
            </select>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-xs text-text-secondary">older than</span>
            <input
              type="number"
              min="1"
              bind:value={metricsDays}
              class="w-20 px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
            />
            <span class="text-xs text-text-secondary">days</span>
          </div>
          <Button size="sm" variant="secondary" onclick={pruneMetrics} disabled={pruningMetrics}>
            {pruningMetrics ? 'Pruning...' : 'Prune'}
          </Button>
        </div>
        {#if breakdown?.metrics}
          <div class="mt-3 flex flex-wrap gap-2">
            {#each breakdown.metrics as s}
              <span class="text-xs px-2 py-0.5 rounded-full bg-surface-1 border border-border/30 text-text-secondary">
                {tierLabels[s.tier] ?? s.tier}: <span class="font-semibold text-text-primary">{s.count.toLocaleString()}</span>
              </span>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Prune Request Stats -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Prune Request Stats</h3>
        <p class="text-xs text-text-secondary mb-4">Delete request stats for a specific resolution tier older than N days.</p>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex items-center gap-2">
            <span class="text-xs text-text-secondary">Tier</span>
            <select
              bind:value={reqStatsTier}
              class="px-2 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
            >
              {#each tiers as t}
                <option value={t}>{tierLabels[t]}</option>
              {/each}
            </select>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-xs text-text-secondary">older than</span>
            <input
              type="number"
              min="1"
              bind:value={reqStatsDays}
              class="w-20 px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
            />
            <span class="text-xs text-text-secondary">days</span>
          </div>
          <Button size="sm" variant="secondary" onclick={pruneReqStats} disabled={pruningReqStats}>
            {pruningReqStats ? 'Pruning...' : 'Prune'}
          </Button>
        </div>
        {#if breakdown?.request_stats}
          <div class="mt-3 flex flex-wrap gap-2">
            {#each breakdown.request_stats as s}
              <span class="text-xs px-2 py-0.5 rounded-full bg-surface-1 border border-border/30 text-text-secondary">
                {tierLabels[s.tier] ?? s.tier}: <span class="font-semibold text-text-primary">{s.count.toLocaleString()}</span>
              </span>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Vacuum -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Vacuum Database</h3>
        <p class="text-xs text-text-secondary mb-4">Reclaim unused space in the SQLite database file. This briefly locks the database.</p>
        <Button size="sm" variant="secondary" onclick={vacuum} disabled={vacuuming}>
          {vacuuming ? 'Running...' : 'Run VACUUM'}
        </Button>
      </div>

      <!-- Database Backup -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Database Backup</h3>
        <p class="text-xs text-text-secondary mb-4">Download or schedule backups of the SimpleDeploy database. Compact mode excludes metrics data.</p>

        <!-- Download Now -->
        <div class="flex flex-wrap items-center gap-3 mb-5 pb-5 border-b border-border/30">
          <label class="flex items-center gap-2 text-xs text-text-secondary">
            <input type="checkbox" bind:checked={downloadCompact} class="rounded border-border accent-accent" />
            Compact (skip metrics)
          </label>
          <Button size="sm" variant="secondary" onclick={downloadBackup} disabled={downloading}>
            {downloading ? 'Downloading...' : 'Download Now'}
          </Button>
        </div>

        <!-- Schedule Config -->
        <div class="space-y-3">
          <div class="flex items-center gap-2">
            <label class="flex items-center gap-2 text-xs text-text-secondary">
              <input type="checkbox" bind:checked={backupConfig.enabled} class="rounded border-border accent-accent" />
              Enable scheduled backup
            </label>
          </div>
          {#if backupConfig.enabled}
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-xs font-medium text-text-secondary mb-1">Cron Schedule</label>
                <input
                  type="text"
                  bind:value={backupConfig.schedule}
                  placeholder="0 2 * * *"
                  class="w-full px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent font-mono"
                />
                <span class="text-xs text-text-muted mt-1 block">e.g. 0 2 * * * = daily at 2 AM</span>
              </div>
              <div>
                <label class="block text-xs font-medium text-text-secondary mb-1">Destination Path</label>
                <input
                  type="text"
                  bind:value={backupConfig.destination}
                  placeholder="/var/backups/simpledeploy"
                  class="w-full px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent font-mono"
                />
              </div>
              <div>
                <label class="block text-xs font-medium text-text-secondary mb-1">Retention Count</label>
                <input
                  type="number"
                  min="1"
                  bind:value={backupConfig.retention}
                  class="w-24 px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
                />
              </div>
              <div class="flex items-end">
                <label class="flex items-center gap-2 text-xs text-text-secondary pb-1.5">
                  <input type="checkbox" bind:checked={backupConfig.compact} class="rounded border-border accent-accent" />
                  Compact (skip metrics)
                </label>
              </div>
            </div>
          {/if}
          <Button size="sm" variant="secondary" onclick={saveBackupConfig} disabled={savingBackupConfig}>
            {savingBackupConfig ? 'Saving...' : 'Save Schedule'}
          </Button>
        </div>

        <!-- Recent Runs -->
        {#if backupRuns.length > 0}
          <div class="mt-5 pt-4 border-t border-border/30">
            <div class="text-xs font-medium text-text-secondary mb-2">Recent Backups</div>
            <div class="space-y-1.5">
              {#each backupRuns.slice(0, 10) as run}
                <div class="flex items-center justify-between text-xs py-1.5 px-2 rounded bg-surface-1/50">
                  <span class="text-text-secondary font-mono">{formatTime(run.created_at)}</span>
                  <span class="text-text-primary font-semibold">{formatBytes(run.size_bytes)}</span>
                  <span class="{run.status === 'ok' ? 'text-emerald-400' : 'text-red-400'}">{run.status}</span>
                  {#if run.compact}<span class="text-text-muted">compact</span>{/if}
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    </div>
  {:else if activeTab === 'audit'}
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-base font-medium text-text-primary">Audit Log</h2>
          <p class="text-xs text-text-secondary mt-1">Security events: logins, user changes, deploys, API key operations.</p>
        </div>
        <div class="flex gap-2">
          <Button size="sm" variant="secondary" onclick={loadAuditLogs} disabled={auditLoading}>
            {auditLoading ? 'Loading...' : 'Refresh'}
          </Button>
          <Button size="sm" variant="danger" onclick={clearAuditLogs} disabled={clearing || auditLogs.length === 0}>
            {clearing ? 'Clearing...' : 'Clear'}
          </Button>
        </div>
      </div>

      <!-- Buffer size config -->
      <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50 flex flex-wrap items-center gap-3">
        <span class="text-xs font-medium text-text-secondary">Buffer limit:</span>
        <input
          type="number"
          min="10"
          max="10000"
          bind:value={auditMaxSize}
          class="w-24 px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
        />
        <span class="text-xs text-text-secondary">events</span>
        <Button size="sm" variant="secondary" onclick={saveAuditConfig} disabled={savingConfig}>
          {savingConfig ? 'Saving...' : 'Save'}
        </Button>
        <span class="text-xs text-text-muted ml-auto">Oldest events auto-pruned when limit is reached</span>
      </div>

      {#if auditLoading && auditLogs.length === 0}
        <Skeleton type="card" count={3} />
      {:else if auditLogs.length === 0}
        <div class="bg-surface-2 rounded-xl p-8 shadow-sm border border-border/50 text-center">
          <p class="text-sm text-text-secondary">No audit events recorded yet.</p>
        </div>
      {:else}
        <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden">
          <div class="overflow-x-auto max-h-[600px] overflow-y-auto">
            <table class="w-full text-sm">
              <thead class="sticky top-0 bg-surface-2 z-10">
                <tr class="border-b border-border/50">
                  <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Time</th>
                  <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Event</th>
                  <th class="text-left text-xs font-medium text-text-muted py-3 px-4">User</th>
                  <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Detail</th>
                  <th class="text-left text-xs font-medium text-text-muted py-3 px-4">IP</th>
                  <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Status</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border/30">
                {#each [...auditLogs].reverse() as event}
                  <tr class="hover:bg-surface-hover transition-colors">
                    <td class="py-2.5 px-4 text-xs text-text-secondary font-mono whitespace-nowrap">{formatTime(event.timestamp)}</td>
                    <td class="py-2.5 px-4">
                      <span class="text-xs font-medium {eventTypeColors[event.type] || 'text-text-primary'}">{event.type}</span>
                    </td>
                    <td class="py-2.5 px-4 text-xs text-text-primary">{event.username || '-'}</td>
                    <td class="py-2.5 px-4 text-xs text-text-secondary font-mono">{event.detail || '-'}</td>
                    <td class="py-2.5 px-4 text-xs text-text-secondary font-mono">{event.ip || '-'}</td>
                    <td class="py-2.5 px-4">
                      {#if event.success}
                        <span class="inline-flex items-center gap-1 text-xs text-emerald-400">
                          <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/></svg>
                          ok
                        </span>
                      {:else}
                        <span class="inline-flex items-center gap-1 text-xs text-red-400">
                          <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"/></svg>
                          fail
                        </span>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          <div class="border-t border-border/50 px-4 py-2">
            <span class="text-xs text-text-secondary">{auditLogs.length} of {auditMaxSize} max events (newest first)</span>
          </div>
        </div>
      {/if}
    </div>
  {:else if activeTab === 'logs'}
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-base font-medium text-text-primary">Process Logs</h2>
          <p class="text-xs text-text-secondary mt-1">SimpleDeploy application logs from the current session.</p>
        </div>
        <div class="flex items-center gap-2">
          <label class="flex items-center gap-2 text-xs text-text-secondary">
            <input type="checkbox" bind:checked={logsAutoScroll} class="rounded border-border accent-accent" />
            Auto-scroll
          </label>
          {#if logsStreaming}
            <Button size="sm" variant="danger" onclick={stopLogsStream}>Stop Stream</Button>
          {:else}
            <Button size="sm" variant="secondary" onclick={startLogsStream}>Live Tail</Button>
          {/if}
          <Button size="sm" variant="secondary" onclick={loadLogs} disabled={logsLoading}>
            {logsLoading ? 'Loading...' : 'Refresh'}
          </Button>
        </div>
      </div>

      {#if logsLoading && processLogs.length === 0}
        <Skeleton type="card" count={5} />
      {:else if processLogs.length === 0}
        <div class="bg-surface-2 rounded-xl p-8 shadow-sm border border-border/50 text-center">
          <p class="text-sm text-text-secondary">No log entries yet.</p>
        </div>
      {:else}
        <div
          id="logs-container"
          class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden"
        >
          <div class="overflow-x-auto max-h-[600px] overflow-y-auto p-4 font-mono text-xs space-y-0.5">
            {#each processLogs as entry}
              <div class="flex gap-3 hover:bg-surface-hover py-0.5 px-1 rounded">
                <span class="text-text-muted whitespace-nowrap shrink-0">{formatTime(entry.timestamp)}</span>
                <span class="text-text-primary break-all">{entry.message}</span>
              </div>
            {/each}
          </div>
          <div class="border-t border-border/50 px-4 py-2 flex items-center justify-between">
            <span class="text-xs text-text-secondary">{processLogs.length} entries</span>
            {#if logsStreaming}
              <span class="text-xs text-emerald-400 flex items-center gap-1">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                Streaming
              </span>
            {/if}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</Layout>
