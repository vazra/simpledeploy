<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import Button from './Button.svelte'
  import Badge from './Badge.svelte'
  import Modal from './Modal.svelte'
  import Skeleton from './Skeleton.svelte'
  import BackupWizard from './BackupWizard.svelte'

  let { slug } = $props()

  let configs = $state([])
  let runs = $state([])
  let loading = $state(true)
  let showWizard = $state(false)
  let showRestoreModal = $state(null)
  let expandedError = $state(null)
  let triggeringId = $state(null)

  let lastRun = $derived(runs[0])

  onMount(loadData)

  async function loadData() {
    loading = true
    const [cfgRes, runRes] = await Promise.all([
      api.listBackupConfigs(slug),
      api.listBackupRuns(slug),
    ])
    configs = cfgRes.data || []
    runs = runRes.data || []
    loading = false
  }

  async function triggerBackup(cfgId) {
    triggeringId = cfgId
    await api.triggerBackupConfig(cfgId)
    triggeringId = null
    await loadData()
  }

  async function deleteConfig(id) {
    await api.deleteBackupConfig(id)
    await loadData()
  }

  async function confirmRestore() {
    await api.restore(showRestoreModal)
    showRestoreModal = null
    await loadData()
  }

  function strategyLabel(s) {
    if (s === 'postgres') return 'Database (PostgreSQL)'
    if (s === 'volume') return 'Files & Volumes'
    return s
  }

  function targetLabel(t, configJson) {
    if (t === 'local') return 'Local storage'
    if (t === 's3') {
      try {
        const cfg = typeof configJson === 'string' ? JSON.parse(configJson) : (configJson || {})
        const bucket = cfg.bucket || cfg.Bucket || ''
        return bucket ? `S3 (${bucket})` : 'S3'
      } catch {
        return 'S3'
      }
    }
    return t
  }

  function cronLabel(expr) {
    if (!expr) return '-'
    const parts = expr.trim().split(/\s+/)
    if (parts.length !== 5) return expr
    const [min, hour, dom, , dow] = parts

    const pad = (n) => String(n).padStart(2, '0')

    // Monthly: specific day of month, any day of week
    if (dom !== '*' && dow === '*') {
      const day = parseInt(dom, 10)
      if (!isNaN(day)) {
        return `Monthly on day ${day} at ${pad(hour)}:${pad(min)}`
      }
    }

    // Weekly: specific days of week
    if (dow !== '*' && dom === '*') {
      const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
      const dayNums = dow.split(',').map(d => parseInt(d, 10))
      if (dayNums.every(d => !isNaN(d) && d >= 0 && d <= 6)) {
        const labels = dayNums.map(d => dayNames[d]).join(', ')
        return `${labels} at ${pad(hour)}:${pad(min)}`
      }
    }

    // Daily: every day at specific time
    if (dow === '*' && dom === '*') {
      return `Daily at ${pad(hour)}:${pad(min)}`
    }

    return expr
  }

  function formatSize(bytes) {
    if (bytes == null || bytes === 0) return '-'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
  }

  function formatDuration(start, end) {
    if (!start || !end) return '-'
    const ms = new Date(end).getTime() - new Date(start).getTime()
    if (ms < 0) return '-'
    if (ms < 1000) return `${ms}ms`
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
    return `${Math.floor(ms / 60000)}m`
  }

  function relativeTime(ts) {
    if (!ts) return '-'
    const diff = Date.now() - new Date(ts).getTime()
    const secs = Math.floor(diff / 1000)
    if (secs < 60) return 'just now'
    const mins = Math.floor(secs / 60)
    if (mins < 60) return `${mins}m ago`
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) return `${hrs}h ago`
    const days = Math.floor(hrs / 24)
    return `${days}d ago`
  }

  function statusVariant(status) {
    if (status === 'success') return 'success'
    if (status === 'failed') return 'danger'
    if (status === 'running') return 'info'
    return 'default'
  }
</script>

{#if loading}
  <Skeleton type="card" count={2} />
{:else if configs.length === 0}
  <!-- Empty state -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 flex flex-col items-center py-16 text-center">
    <svg class="w-12 h-12 text-text-muted mb-4" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5M10 11.25h4M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-.375c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v.375c0 .621.504 1.125 1.125 1.125z" />
    </svg>
    <h3 class="text-base font-semibold text-text-primary mb-2">No backups configured</h3>
    <p class="text-sm text-text-muted max-w-md mb-6">
      Set up your first backup to protect your data. SimpleDeploy can automatically back up your databases and file volumes on a schedule.
    </p>
    <Button onclick={() => showWizard = true}>Configure Backup</Button>
  </div>
{:else}
  <div class="space-y-4">
    <!-- Status header card -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 flex items-center justify-between gap-4 flex-wrap">
      <div class="flex items-center gap-3">
        <span class="text-sm text-text-secondary">Last backup:</span>
        {#if lastRun}
          <span class="text-sm font-medium text-text-primary">{relativeTime(lastRun.finished_at || lastRun.started_at)}</span>
          <Badge variant={statusVariant(lastRun.status)}>{lastRun.status}</Badge>
        {:else}
          <span class="text-sm text-text-muted">Never</span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        {#if configs.length === 1}
          <Button
            variant="secondary"
            size="sm"
            loading={triggeringId === configs[0].id}
            onclick={() => triggerBackup(configs[0].id)}
          >
            Backup Now
          </Button>
        {:else}
          <!-- Multi-config dropdown on hover -->
          <div class="relative group">
            <Button variant="secondary" size="sm">
              Backup Now
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
              </svg>
            </Button>
            <div class="absolute right-0 top-full mt-1 w-52 bg-surface-2 border border-border/50 rounded-xl shadow-lg z-10 hidden group-hover:block">
              {#each configs as cfg}
                <button
                  class="w-full text-left px-4 py-2.5 text-sm text-text-secondary hover:text-text-primary hover:bg-surface-3 first:rounded-t-xl last:rounded-b-xl transition-colors disabled:opacity-50"
                  disabled={triggeringId === cfg.id}
                  onclick={() => triggerBackup(cfg.id)}
                >
                  {strategyLabel(cfg.strategy)}
                </button>
              {/each}
            </div>
          </div>
        {/if}
        <Button size="sm" onclick={() => showWizard = true}>
          Add Config
        </Button>
      </div>
    </div>

    <!-- Backup Configurations card -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
      <h3 class="text-sm font-medium text-text-primary mb-4">Backup Configurations</h3>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-border/30">
              <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Type</th>
              <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Destination</th>
              <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Schedule</th>
              <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Retention</th>
              <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/30">
            {#each configs as cfg}
              <tr class="hover:bg-surface-hover">
                <td class="py-2.5 px-3 text-text-primary">{strategyLabel(cfg.strategy)}</td>
                <td class="py-2.5 px-3 text-text-secondary">{targetLabel(cfg.target, cfg.target_config_json)}</td>
                <td class="py-2.5 px-3 text-text-secondary">{cronLabel(cfg.schedule_cron)}</td>
                <td class="py-2.5 px-3 text-text-secondary">
                  {#if cfg.retention_count}
                    Keep last {cfg.retention_count}
                  {:else}
                    -
                  {/if}
                </td>
                <td class="py-2.5 px-3">
                  <Button variant="ghost" size="sm" onclick={() => deleteConfig(cfg.id)}>Delete</Button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <!-- Backup History card -->
    {#if runs.length > 0}
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h3 class="text-sm font-medium text-text-primary mb-4">Backup History</h3>
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border/30">
                <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Status</th>
                <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Size</th>
                <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Duration</th>
                <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Time</th>
                <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each runs as run}
                <tr class="border-b border-border/30 hover:bg-surface-hover">
                  <td class="py-2.5 px-3">
                    <Badge variant={statusVariant(run.status)}>{run.status}</Badge>
                  </td>
                  <td class="py-2.5 px-3 text-text-secondary">{formatSize(run.size_bytes)}</td>
                  <td class="py-2.5 px-3 text-text-secondary">{formatDuration(run.started_at, run.finished_at)}</td>
                  <td class="py-2.5 px-3 text-text-secondary">
                    <span title={run.finished_at || run.started_at ? new Date(run.finished_at || run.started_at).toLocaleString() : ''}>
                      {relativeTime(run.finished_at || run.started_at)}
                    </span>
                  </td>
                  <td class="py-2.5 px-3">
                    <div class="flex items-center gap-2">
                      {#if run.status === 'success'}
                        <Button variant="ghost" size="sm" onclick={() => showRestoreModal = run.id}>Restore</Button>
                      {/if}
                      {#if run.status === 'failed' && (run.error_msg || run.error)}
                        <Button
                          variant="ghost"
                          size="sm"
                          onclick={() => expandedError = expandedError === run.id ? null : run.id}
                        >
                          {expandedError === run.id ? 'Hide Error' : 'View Error'}
                        </Button>
                      {/if}
                    </div>
                  </td>
                </tr>
                {#if run.status === 'failed' && expandedError === run.id}
                  <tr class="border-b border-border/30">
                    <td colspan="5" class="px-3 py-3">
                      <pre class="text-xs font-mono whitespace-pre-wrap break-all bg-danger/5 text-danger/80 rounded-lg p-3 max-h-40 overflow-y-auto">{run.error_msg || run.error}</pre>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  </div>
{/if}

<BackupWizard open={showWizard} {slug} onclose={() => showWizard = false} oncreated={loadData} />

{#if showRestoreModal}
  <Modal
    title="Restore Backup"
    message="This will restore the app from this backup. The current state will be overwritten. Continue?"
    onConfirm={confirmRestore}
    onCancel={() => showRestoreModal = null}
  />
{/if}
