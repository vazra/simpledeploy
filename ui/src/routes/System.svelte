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

  let metricsDays = $state(30)
  let reqStatsDays = $state(30)
  let pruningMetrics = $state(false)
  let pruningReqStats = $state(false)
  let vacuuming = $state(false)

  function formatBytes(bytes) {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  onMount(() => {
    loadInfo()
  })

  async function loadInfo() {
    loading = true
    const res = await api.systemInfo()
    if (res.data) info = res.data
    loading = false
  }

  function switchTab(tab) {
    activeTab = tab
  }

  async function pruneMetrics() {
    pruningMetrics = true
    const res = await api.systemPruneMetrics(metricsDays)
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success(`Metrics older than ${metricsDays} days pruned`)
    }
    pruningMetrics = false
  }

  async function pruneReqStats() {
    pruningReqStats = true
    const res = await api.systemPruneRequestStats(reqStatsDays)
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success(`Request stats older than ${reqStatsDays} days pruned`)
    }
    pruningReqStats = false
  }

  async function vacuum() {
    vacuuming = true
    const res = await api.systemVacuum()
    if (res.error) {
      toasts.error(res.error)
    } else {
      toasts.success('VACUUM completed successfully')
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
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">System</h1>
  </div>

  <div class="flex gap-1 mb-6 border-b border-border">
    {#each [['overview', 'Overview'], ['maintenance', 'Maintenance']] as [key, label]}
      <button
        onclick={() => switchTab(key)}
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === key ? 'border-accent text-accent' : 'border-transparent text-text-secondary hover:text-text-primary'}"
      >{label}</button>
    {/each}
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={3} />
    </div>
  {:else if activeTab === 'overview'}
    {#if info}
      <!-- SimpleDeploy section -->
      <h2 class="text-sm font-semibold text-text-primary mb-3">SimpleDeploy</h2>
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-6">
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

      <!-- System Resources section -->
      <h2 class="text-sm font-semibold text-text-primary mb-3">System Resources</h2>
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-6">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-6">
          <div>
            <div class="text-xs font-medium text-text-secondary mb-1">CPU Cores</div>
            <div class="text-xl font-bold text-text-primary">{info.resources?.cpu_count ?? '-'}</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary mb-1">RAM</div>
            <div class="text-sm font-semibold text-text-primary">{formatBytes(info.resources?.ram_used || 0)} used</div>
            <div class="text-xs text-text-secondary">{formatBytes(info.resources?.ram_avail || 0)} free / {formatBytes(info.resources?.ram_total || 0)} total</div>
          </div>
          <div>
            <div class="text-xs font-medium text-text-secondary mb-2">Disk</div>
            <div class="flex items-center gap-2 mb-1">
              <div class="flex-1 bg-surface-1 rounded-full h-2 overflow-hidden">
                <div
                  class="h-2 rounded-full transition-all {(info.resources?.disk_used_pct || 0) > 85 ? 'bg-red-500' : (info.resources?.disk_used_pct || 0) > 70 ? 'bg-yellow-500' : 'bg-accent'}"
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

      <!-- Database section -->
      <h2 class="text-sm font-semibold text-text-primary mb-3">Database</h2>
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-6">
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
              <tbody class="divide-y divide-border-muted">
                {#each Object.entries(rowCountLabels) as [key, label]}
                  <tr class="hover:bg-surface-1">
                    <td class="py-1.5 text-text-secondary text-xs">{label}</td>
                    <td class="py-1.5 text-right font-semibold text-text-primary text-xs">{(info.database.row_counts[key] ?? 0).toLocaleString()}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

      <!-- Apps section -->
      <h2 class="text-sm font-semibold text-text-primary mb-3">Apps</h2>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="text-xs font-medium text-text-secondary mb-1">Total</div>
          <div class="text-2xl font-bold text-text-primary">{info.apps?.total ?? 0}</div>
        </div>
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="text-xs font-medium text-text-secondary mb-1">Running</div>
          <div class="text-2xl font-bold text-green-500">{info.apps?.running ?? 0}</div>
        </div>
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="text-xs font-medium text-text-secondary mb-1">Stopped</div>
          <div class="text-2xl font-bold text-text-secondary">{info.apps?.stopped ?? 0}</div>
        </div>
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="text-xs font-medium text-text-secondary mb-1">Error</div>
          <div class="text-2xl font-bold text-red-500">{info.apps?.error ?? 0}</div>
        </div>
      </div>
    {:else}
      <p class="text-sm text-text-secondary">Failed to load system info.</p>
    {/if}

  {:else if activeTab === 'maintenance'}
    <div class="space-y-4">
      <!-- Prune Metrics -->
      <div class="bg-surface-2 border border-border rounded-lg p-4">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Prune Metrics</h3>
        <p class="text-xs text-text-secondary mb-4">Delete raw metrics data older than N days to free up database space.</p>
        <div class="flex items-center gap-3">
          <input
            type="number"
            min="1"
            bind:value={metricsDays}
            class="w-24 px-3 py-1.5 text-sm bg-surface-1 border border-border rounded-md text-text-primary focus:outline-none focus:border-accent"
          />
          <span class="text-xs text-text-secondary">days</span>
          <Button size="sm" variant="secondary" onclick={pruneMetrics} disabled={pruningMetrics}>
            {pruningMetrics ? 'Pruning...' : 'Prune Metrics'}
          </Button>
        </div>
      </div>

      <!-- Prune Request Stats -->
      <div class="bg-surface-2 border border-border rounded-lg p-4">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Prune Request Stats</h3>
        <p class="text-xs text-text-secondary mb-4">Delete raw request stats data older than N days to free up database space.</p>
        <div class="flex items-center gap-3">
          <input
            type="number"
            min="1"
            bind:value={reqStatsDays}
            class="w-24 px-3 py-1.5 text-sm bg-surface-1 border border-border rounded-md text-text-primary focus:outline-none focus:border-accent"
          />
          <span class="text-xs text-text-secondary">days</span>
          <Button size="sm" variant="secondary" onclick={pruneReqStats} disabled={pruningReqStats}>
            {pruningReqStats ? 'Pruning...' : 'Prune Request Stats'}
          </Button>
        </div>
      </div>

      <!-- Vacuum -->
      <div class="bg-surface-2 border border-border rounded-lg p-4">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Vacuum Database</h3>
        <p class="text-xs text-text-secondary mb-4">Reclaim unused space in the SQLite database file. This briefly locks the database.</p>
        <Button size="sm" variant="secondary" onclick={vacuum} disabled={vacuuming}>
          {vacuuming ? 'Running...' : 'Run VACUUM'}
        </Button>
      </div>
    </div>
  {/if}
</Layout>
