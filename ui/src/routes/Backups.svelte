<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Badge from '../components/Badge.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import BackupHealthCard from '../components/BackupHealthCard.svelte'
  import { api } from '../lib/api.js'
  import { realtime } from '../lib/stores/realtime.svelte.js'

  let apps = $state([])
  let recentRuns = $state([])
  let loading = $state(true)
  let statusFilter = $state('all')

  onMount(() => {
    loadSummary()
    const offB = realtime.register('global:backups', loadSummary)
    const offA = realtime.register('global:apps', loadSummary)
    return () => { offB(); offA() }
  })

  async function loadSummary() {
    const res = await api.backupSummary()
    apps = res.data?.apps || []
    recentRuns = res.data?.recent_runs || []
    loading = false
  }

  const totalConfigs = $derived(apps.reduce((s, a) => s + (a.config_count || 0), 0))
  const totalSuccess24h = $derived(apps.reduce((s, a) => s + (a.recent_success_count || 0), 0))
  const totalFail24h = $derived(apps.reduce((s, a) => s + (a.recent_fail_count || 0), 0))
  const totalStorage = $derived(apps.reduce((s, a) => s + (a.total_size_bytes || 0), 0))
  const totalMissed24h = $derived(apps.reduce((s, a) => s + (a.missed_count || 0), 0))
  const filteredRuns = $derived(
    statusFilter === 'all' ? recentRuns : recentRuns.filter(r => r.status === statusFilter)
  )

  function formatSize(bytes) {
    if (!bytes || bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
  }

  function relativeTime(ts) {
    if (!ts) return '-'
    const diff = Date.now() - new Date(ts).getTime()
    const s = Math.floor(diff / 1000)
    const m = Math.floor(s / 60)
    const h = Math.floor(m / 60)
    const d = Math.floor(h / 24)
    if (s < 60) return 'just now'
    if (m < 60) return `${m}m ago`
    if (h < 24) return `${h}h ago`
    if (d < 365) return `${d}d ago`
    return '-'
  }

  function formatDuration(start, end) {
    if (!start || !end) return '-'
    const ms = new Date(end).getTime() - new Date(start).getTime()
    if (ms < 0) return '-'
    const s = Math.floor(ms / 1000)
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    return `${m}m ${s % 60}s`
  }

  function strategyLabel(s) {
    if (s === 'postgres') return 'Database'
    if (s === 'volume') return 'Files'
    return s || '-'
  }

  function statusVariant(status) {
    if (status === 'success') return 'success'
    if (status === 'failed') return 'danger'
    if (status === 'running') return 'warning'
    return 'default'
  }

  const filters = ['all', 'success', 'failed', 'running']
</script>

<Layout>
  <div class="mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Backups</h1>
    <p class="text-sm text-text-muted mt-0.5">Overview of all backup activity</p>
  </div>

  {#if loading}
    <Skeleton type="card" count={3} />
  {:else if apps.length === 0}
    <div class="bg-surface-2 rounded-xl p-8 shadow-sm border border-border/50 text-center">
      <p class="text-sm font-medium text-text-primary mb-1">No backup configurations found across any app.</p>
      <p class="text-sm text-text-muted">Configure backups from an app's Backups tab.</p>
    </div>
  {:else}
    <!-- Summary stats -->
    <div class="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
      <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50">
        <div class="text-xs text-text-muted mb-1">Total Configs</div>
        <div class="text-2xl font-semibold text-text-primary">{totalConfigs}</div>
      </div>
      <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50">
        <div class="text-xs text-text-muted mb-1">24h Successful</div>
        <div class="text-2xl font-semibold text-success">{totalSuccess24h}</div>
      </div>
      <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50">
        <div class="text-xs text-text-muted mb-1">24h Failed</div>
        <div class="text-2xl font-semibold {totalFail24h > 0 ? 'text-danger' : 'text-text-primary'}">{totalFail24h}</div>
      </div>
      <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50">
        <div class="text-xs text-text-muted mb-1">Missed (24h)</div>
        <div class="text-2xl font-semibold {totalMissed24h > 0 ? 'text-warning' : 'text-text-primary'}">{totalMissed24h}</div>
      </div>
      <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50">
        <div class="text-xs text-text-muted mb-1">Total Storage</div>
        <div class="text-2xl font-semibold text-text-primary">{formatSize(totalStorage)}</div>
      </div>
    </div>

    <!-- Per-app health cards -->
    <h2 class="text-sm font-semibold text-text-primary mb-3">Apps</h2>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-3 mb-6">
      {#each apps as app}
        <BackupHealthCard {app} />
      {/each}
    </div>

    <!-- Recent activity -->
    <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50">
      <div class="flex items-center justify-between px-5 py-4 border-b border-border/50">
        <h2 class="text-sm font-semibold text-text-primary">Recent Activity</h2>
        <div class="flex items-center gap-1">
          {#each filters as f}
            <button
              onclick={() => statusFilter = f}
              class="px-2.5 py-1 text-[11px] rounded-md transition-colors capitalize {statusFilter === f ? 'bg-surface-3 text-text-primary' : 'text-text-muted hover:text-text-primary'}"
            >{f}</button>
          {/each}
        </div>
      </div>

      {#if filteredRuns.length === 0}
        <p class="text-sm text-text-muted px-5 py-6">No backup runs{statusFilter !== 'all' ? ` with status "${statusFilter}"` : ''}.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">App</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Type</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Status</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Size</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Duration</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Time</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/30">
              {#each filteredRuns as run}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4">
                    <a href="#/apps/{run.app_slug}?tab=backups" class="text-accent hover:underline">{run.app_name || run.app_slug}</a>
                  </td>
                  <td class="py-3 px-4 text-text-secondary">{strategyLabel(run.strategy)}</td>
                  <td class="py-3 px-4"><Badge variant={statusVariant(run.status)}>{run.status}</Badge></td>
                  <td class="py-3 px-4 text-text-secondary">{formatSize(run.size_bytes)}</td>
                  <td class="py-3 px-4 text-text-secondary">{formatDuration(run.started_at, run.finished_at)}</td>
                  <td class="py-3 px-4 text-text-muted">{relativeTime(run.started_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}
</Layout>
