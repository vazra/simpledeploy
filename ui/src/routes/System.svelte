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

  function switchTab(tab) {
    activeTab = tab
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
    {#each [['overview', 'Overview'], ['maintenance', 'Maintenance']] as [key, label]}
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
    </div>
  {/if}
</Layout>
