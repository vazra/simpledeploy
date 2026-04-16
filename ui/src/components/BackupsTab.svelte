<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import Button from './Button.svelte'
  import Badge from './Badge.svelte'
  import Modal from './Modal.svelte'
  import Skeleton from './Skeleton.svelte'
  import BackupWizard from './BackupWizard.svelte'
  import { toasts } from '../lib/stores/toast.js'

  let { slug } = $props()

  let configs = $state([])
  let runs = $state([])
  let loading = $state(true)
  let showWizard = $state(false)
  let editConfig = $state(null)
  let showRestoreModal = $state(null)
  let showUploadModal = $state(false)
  let expandedError = $state(null)
  let expandedChecksum = $state(null)
  let triggeringId = $state(null)

  // Upload restore state
  let uploadStrategy = $state('')
  let uploadContainer = $state('')
  let uploadFile = $state(null)
  let uploading = $state(false)

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

  function openEditWizard(cfg) {
    editConfig = cfg
    showWizard = true
  }

  function openCreateWizard() {
    editConfig = null
    showWizard = true
  }

  function closeWizard() {
    showWizard = false
    editConfig = null
  }

  async function confirmRestore() {
    await api.restore(showRestoreModal)
    showRestoreModal = null
    await loadData()
  }

  function openUploadModal() {
    uploadStrategy = ''
    uploadContainer = ''
    uploadFile = null
    uploading = false
    showUploadModal = true
  }

  async function submitUploadRestore() {
    if (!uploadFile || !uploadStrategy) return
    uploading = true
    const formData = new FormData()
    formData.append('file', uploadFile)
    formData.append('strategy', uploadStrategy)
    if (uploadContainer) formData.append('container', uploadContainer)
    const res = await api.uploadRestore(slug, formData)
    uploading = false
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success('Restore from file started')
      showUploadModal = false
      await loadData()
    }
  }

  function strategyLabel(s) {
    const labels = {
      postgres: 'Database (PostgreSQL)',
      mysql: 'Database (MySQL)',
      redis: 'Redis',
      volume: 'Files & Volumes',
      sqlite: 'SQLite Database',
      mongo: 'Database (MongoDB)',
    }
    return labels[s] || s
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

    if (dom !== '*' && dow === '*') {
      const day = parseInt(dom, 10)
      if (!isNaN(day)) {
        return `Monthly on day ${day} at ${pad(hour)}:${pad(min)}`
      }
    }

    if (dow !== '*' && dom === '*') {
      const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
      const dayNums = dow.split(',').map(d => parseInt(d, 10))
      if (dayNums.every(d => !isNaN(d) && d >= 0 && d <= 6)) {
        const labels = dayNums.map(d => dayNames[d]).join(', ')
        return `${labels} at ${pad(hour)}:${pad(min)}`
      }
    }

    if (dow === '*' && dom === '*') {
      return `Daily at ${pad(hour)}:${pad(min)}`
    }

    return expr
  }

  function retentionLabel(cfg) {
    if (cfg.retention_mode === 'days' && cfg.retention_days) return `${cfg.retention_days} days`
    if (cfg.retention_count) return `Keep last ${cfg.retention_count}`
    return '-'
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

  function truncateChecksum(hash) {
    if (!hash) return ''
    return hash.length > 16 ? hash.slice(0, 16) + '...' : hash
  }

  function statusVariant(status) {
    if (status === 'success') return 'success'
    if (status === 'failed') return 'danger'
    if (status === 'running') return 'info'
    return 'default'
  }

  const strategyOptions = ['postgres', 'mysql', 'redis', 'volume', 'sqlite', 'mongo']
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
    <Button onclick={openCreateWizard}>Configure Backup</Button>
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
        <Button variant="secondary" size="sm" onclick={openUploadModal}>
          Restore from File
        </Button>
        <Button size="sm" onclick={openCreateWizard}>
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
                <td class="py-2.5 px-3 text-text-secondary">{retentionLabel(cfg)}</td>
                <td class="py-2.5 px-3">
                  <div class="flex items-center gap-1">
                    <Button variant="ghost" size="sm" onclick={() => triggerBackup(cfg.id)} loading={triggeringId === cfg.id}>Run</Button>
                    <Button variant="ghost" size="sm" onclick={() => openEditWizard(cfg)}>Edit</Button>
                    <Button variant="ghost" size="sm" onclick={() => deleteConfig(cfg.id)}>Delete</Button>
                  </div>
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
                <th class="text-left text-xs font-medium text-text-muted py-2.5 px-3">Checksum</th>
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
                  <td class="py-2.5 px-3 text-text-muted">
                    {#if run.checksum}
                      <button
                        type="button"
                        onclick={() => expandedChecksum = expandedChecksum === run.id ? null : run.id}
                        class="font-mono text-xs hover:text-text-primary transition-colors"
                        title="Click to expand"
                      >
                        {expandedChecksum === run.id ? run.checksum : truncateChecksum(run.checksum)}
                      </button>
                    {:else}
                      <span class="text-xs">-</span>
                    {/if}
                  </td>
                  <td class="py-2.5 px-3">
                    <div class="flex items-center gap-2">
                      {#if run.status === 'success'}
                        <Button variant="ghost" size="sm" onclick={() => showRestoreModal = run.id}>Restore</Button>
                        <a
                          href={api.downloadBackupUrl(run.id)}
                          download
                          class="px-2 py-1 text-xs text-text-secondary hover:text-text-primary transition-colors"
                        >Download</a>
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
                    <td colspan="6" class="px-3 py-3">
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

<BackupWizard open={showWizard} {slug} {editConfig} onclose={closeWizard} oncreated={loadData} />

{#if showRestoreModal}
  <Modal
    title="Restore Backup"
    message="This will restore the app from this backup. The current state will be overwritten. Continue?"
    onConfirm={confirmRestore}
    onCancel={() => showRestoreModal = null}
  />
{/if}

{#if showUploadModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4" role="dialog" aria-modal="true">
    <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={() => showUploadModal = false} aria-label="Close"></button>
    <div class="relative bg-surface-2 border border-border/50 rounded-2xl shadow-2xl animate-scale-in max-w-md w-full">
      <div class="flex items-center justify-between px-6 py-4 border-b border-border/30">
        <h3 class="text-lg font-semibold text-text-primary tracking-tight">Restore from File</h3>
        <button onclick={() => showUploadModal = false} class="text-text-muted hover:text-text-primary transition-colors p-1 rounded-lg hover:bg-surface-3" aria-label="Close">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <div class="p-6 space-y-4">
        <div>
          <label class="block text-xs font-medium text-text-secondary mb-1">
            Strategy <span class="text-danger">*</span>
          </label>
          <select
            class="w-full bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50"
            bind:value={uploadStrategy}
          >
            <option value="">Select strategy...</option>
            {#each strategyOptions as opt}
              <option value={opt}>{strategyLabel(opt)}</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium text-text-secondary mb-1">Container (optional)</label>
          <input
            type="text"
            class="w-full bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50"
            placeholder="e.g. postgres"
            bind:value={uploadContainer}
          />
          <p class="text-xs text-text-muted mt-1">Name of the service to restore into</p>
        </div>
        <div>
          <label class="block text-xs font-medium text-text-secondary mb-1">
            Backup file <span class="text-danger">*</span>
          </label>
          <input
            type="file"
            class="w-full text-sm text-text-secondary file:mr-3 file:py-2 file:px-3 file:rounded-lg file:border-0 file:text-sm file:bg-surface-3 file:text-text-primary hover:file:bg-surface-3/80"
            onchange={(e) => uploadFile = e.target.files[0]}
          />
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <Button variant="secondary" size="sm" onclick={() => showUploadModal = false}>Cancel</Button>
          <Button
            size="sm"
            loading={uploading}
            disabled={!uploadStrategy || !uploadFile || uploading}
            onclick={submitUploadRestore}
          >
            Restore
          </Button>
        </div>
      </div>
    </div>
  </div>
{/if}
