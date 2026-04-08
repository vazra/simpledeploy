<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import AppCard from '../components/AppCard.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import { api } from '../lib/api.js'

  let apps = $state([])
  let systemMetrics = $state(null)
  let cpuHistory = $state([])
  let memHistory = $state([])
  let loading = $state(true)

  onMount(async () => {
    try {
      const now = new Date().toISOString()
      const hourAgo = new Date(Date.now() - 3600000).toISOString()

      const [appsData, metricsData] = await Promise.all([
        api.listApps(),
        api.systemMetrics(hourAgo, now).catch(() => [])
      ])

      apps = appsData || []

      if (metricsData && metricsData.length > 0) {
        const latest = metricsData[metricsData.length - 1]
        systemMetrics = {
          cpu: latest.cpu_pct?.toFixed(1) || '0',
          memUsed: formatBytes(latest.mem_bytes || 0),
          memTotal: formatBytes(latest.mem_limit || 0),
          memPct: latest.mem_limit ? ((latest.mem_bytes / latest.mem_limit) * 100).toFixed(1) : '0'
        }
        cpuHistory = metricsData.map(m => ({ x: new Date(m.timestamp), y: m.cpu_pct }))
        memHistory = metricsData.map(m => ({
          x: new Date(m.timestamp),
          y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0
        }))
      }
    } catch (e) {
      console.error('Dashboard load error:', e)
    } finally {
      loading = false
    }
  })

  function formatBytes(bytes) {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
  }

  let runningCount = $derived(apps.filter(a => a.Status === 'running').length)
  let stoppedCount = $derived(apps.filter(a => a.Status !== 'running').length)
</script>

<Layout>
  {#if loading}
    <div class="loading">Loading dashboard...</div>
  {:else}
    <div class="stats-row">
      <div class="stat-card">
        <span class="stat-label">CPU</span>
        <span class="stat-value">{systemMetrics?.cpu || '0'}%</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Memory</span>
        <span class="stat-value">{systemMetrics?.memPct || '0'}%</span>
        {#if systemMetrics}
          <span class="stat-sub">{systemMetrics.memUsed} / {systemMetrics.memTotal}</span>
        {/if}
      </div>
      <div class="stat-card">
        <span class="stat-label">Apps</span>
        <span class="stat-value">{apps.length}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Running</span>
        <span class="stat-value green">{runningCount}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Stopped</span>
        <span class="stat-value muted">{stoppedCount}</span>
      </div>
    </div>

    {#if cpuHistory.length > 0}
      <div class="charts-row">
        <MetricsChart data={cpuHistory} label="CPU Usage" color="#58a6ff" unit="%" />
        <MetricsChart data={memHistory} label="Memory Usage" color="#3fb950" unit="%" />
      </div>
    {/if}

    <h2 class="section-title">Applications</h2>
    {#if apps.length === 0}
      <p class="empty">No apps deployed yet.</p>
    {:else}
      <div class="app-grid">
        {#each apps as app}
          <AppCard {app} />
        {/each}
      </div>
    {/if}
  {/if}
</Layout>

<style>
  .loading {
    color: #8b949e;
    padding: 2rem;
    text-align: center;
  }
  .stats-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 0.75rem;
    margin-bottom: 1.25rem;
  }
  .stat-card {
    background: #1c1f26;
    border: 1px solid #2d3139;
    border-radius: 8px;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .stat-label {
    font-size: 0.75rem;
    color: #8b949e;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .stat-value {
    font-size: 1.5rem;
    font-weight: 600;
    color: #e1e4e8;
  }
  .stat-value.green { color: #3fb950; }
  .stat-value.muted { color: #8b949e; }
  .stat-sub {
    font-size: 0.72rem;
    color: #8b949e;
  }
  .charts-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
    margin-bottom: 1.25rem;
  }
  .section-title {
    font-size: 1rem;
    font-weight: 600;
    color: #e1e4e8;
    margin: 0 0 0.75rem;
  }
  .empty {
    color: #8b949e;
    font-size: 0.85rem;
  }
  .app-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 0.75rem;
  }
</style>
