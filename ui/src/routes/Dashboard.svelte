<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import AppCard from '../components/AppCard.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import Badge from '../components/Badge.svelte'
  import { api } from '../lib/api.js'

  let apps = $state([])
  let cpuHistory = $state([])
  let memHistory = $state([])
  let loading = $state(true)
  let latestMetrics = $state(null)
  let alertRules = $state([])
  let alertHistory = $state([])
  let backupRunsByApp = $state({})
  let appMetricsMap = $state({})
  let appRequestsMap = $state({})
  let timeRange = $state('1h')

  const rangeMs = { '1h': 3600000, '6h': 21600000, '24h': 86400000, '7d': 604800000 }

  let filterStatus = $state('all')
  let sortBy = $state('name')

  onMount(loadDashboard)

  async function loadDashboard() {
    loading = true
    const now = new Date().toISOString()
    const from = new Date(Date.now() - rangeMs[timeRange]).toISOString()

    const [appsRes, metricsRes, rulesRes, histRes] = await Promise.all([
      api.listApps(),
      api.systemMetrics(from, now),
      api.listAlertRules(),
      api.alertHistory(),
    ])

    apps = appsRes.data || []
    alertRules = rulesRes.data || []
    alertHistory = histRes.data || []

    const metricsData = metricsRes.data || []
    if (metricsData.length > 0) {
      const latest = metricsData[metricsData.length - 1]
      latestMetrics = {
        cpu: latest.cpu_pct?.toFixed(1) || '0',
        memUsed: formatBytes(latest.mem_bytes || 0),
        memTotal: formatBytes(latest.mem_limit || 0),
        memPct: latest.mem_limit ? ((latest.mem_bytes / latest.mem_limit) * 100).toFixed(1) : '0',
        netRx: formatBytes(latest.net_rx || 0),
        netTx: formatBytes(latest.net_tx || 0),
        diskRead: formatBytes(latest.disk_read || 0),
        diskWrite: formatBytes(latest.disk_write || 0),
      }
      cpuHistory = metricsData.map((m) => ({ x: new Date(m.timestamp), y: m.cpu_pct }))
      memHistory = metricsData.map((m) => ({
        x: new Date(m.timestamp),
        y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0,
      }))
    }

    // Load per-app metrics and request stats
    const hourAgo = new Date(Date.now() - 3600000).toISOString()
    await Promise.all(
      apps.map(async (app) => {
        const slug = app.Slug || app.slug
        const [mRes, rRes, bRes] = await Promise.all([
          api.appMetrics(slug, hourAgo, now),
          api.appRequests(slug, hourAgo, now),
          api.listBackupRuns(slug),
        ])
        if (mRes.data && mRes.data.length > 0) {
          const latest = mRes.data[mRes.data.length - 1]
          appMetricsMap[slug] = {
            cpu: latest.cpu_pct,
            memPct: latest.mem_limit ? (latest.mem_bytes / latest.mem_limit) * 100 : 0,
          }
        }
        if (rRes.data) {
          appRequestsMap[slug] = rRes.data
        }
        if (bRes.data && bRes.data.length > 0) {
          backupRunsByApp[slug] = bRes.data
        }
      })
    )
    appMetricsMap = { ...appMetricsMap }
    appRequestsMap = { ...appRequestsMap }
    backupRunsByApp = { ...backupRunsByApp }

    loading = false
  }

  function formatBytes(bytes) {
    if (!bytes) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
  }

  function formatTime(ts) {
    if (!ts) return ''
    const d = new Date(ts)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  function formatDate(ts) {
    if (!ts) return ''
    return new Date(ts).toLocaleString()
  }

  let runningCount = $derived(apps.filter((a) => a.Status === 'running').length)
  let stoppedCount = $derived(apps.filter((a) => a.Status !== 'running').length)

  let activeAlerts = $derived((alertHistory || []).filter((h) => !h.resolved_at))

  let recentBackups = $derived(() => {
    const all = []
    for (const [slug, runs] of Object.entries(backupRunsByApp)) {
      for (const run of runs.slice(0, 3)) {
        all.push({ ...run, slug })
      }
    }
    return all.sort((a, b) => new Date(b.started_at) - new Date(a.started_at)).slice(0, 5)
  })

  let filteredApps = $derived(() => {
    let result = apps
    if (filterStatus !== 'all') {
      result = result.filter((a) => a.Status === filterStatus)
    }
    if (sortBy === 'name') {
      result = [...result].sort((a, b) => (a.Name || '').localeCompare(b.Name || ''))
    } else if (sortBy === 'status') {
      result = [...result].sort((a, b) => (a.Status || '').localeCompare(b.Status || ''))
    } else if (sortBy === 'cpu') {
      result = [...result].sort((a, b) => {
        const aCpu = appMetricsMap[a.Slug]?.cpu || 0
        const bCpu = appMetricsMap[b.Slug]?.cpu || 0
        return bCpu - aCpu
      })
    }
    return result
  })
</script>

<Layout>
  {#if loading}
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      <Skeleton type="card" count={4} />
    </div>
    <div class="grid grid-cols-3 gap-3 mb-4">
      <Skeleton type="card" count={3} />
    </div>
    <div class="grid grid-cols-2 gap-3">
      <Skeleton type="chart" count={2} />
    </div>
  {:else}
    <!-- System Health -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      <StatCard
        label="CPU"
        value="{latestMetrics?.cpu || '0'}%"
        color={parseFloat(latestMetrics?.cpu || 0) > 80 ? 'text-danger' : parseFloat(latestMetrics?.cpu || 0) > 50 ? 'text-warning' : 'text-success'}
      />
      <StatCard
        label="Memory"
        value="{latestMetrics?.memPct || '0'}%"
        sub="{latestMetrics?.memUsed || '0'} / {latestMetrics?.memTotal || '0'}"
        color={parseFloat(latestMetrics?.memPct || 0) > 80 ? 'text-danger' : parseFloat(latestMetrics?.memPct || 0) > 50 ? 'text-warning' : 'text-success'}
      />
      <StatCard label="Network" value="{latestMetrics?.netRx || '0 B'}/s" sub="TX: {latestMetrics?.netTx || '0 B'}/s" />
      <StatCard label="Disk I/O" value="{latestMetrics?.diskRead || '0 B'}/s" sub="Write: {latestMetrics?.diskWrite || '0 B'}/s" />
    </div>

    <!-- App Summary -->
    <div class="grid grid-cols-3 gap-3 mb-4">
      <StatCard label="Total Apps" value={apps.length} />
      <StatCard label="Running" value={runningCount} color="text-success" />
      <button onclick={() => filterStatus = filterStatus === 'stopped' ? 'all' : 'stopped'} class="text-left">
        <StatCard label="Stopped / Error" value={stoppedCount} color={stoppedCount > 0 ? 'text-danger' : 'text-text-secondary'} />
      </button>
    </div>

    <!-- Main Content: Apps + Sidebar panels -->
    <div class="grid grid-cols-1 xl:grid-cols-5 gap-4 mb-4">
      <!-- Apps Grid (3/5) -->
      <div class="xl:col-span-3">
        <div class="flex items-center justify-between mb-3">
          <h2 class="text-base font-semibold text-text-primary">Applications</h2>
          <div class="flex items-center gap-2">
            <select
              bind:value={filterStatus}
              class="text-xs bg-surface-2 border border-border rounded-md px-2 py-1 text-text-secondary"
            >
              <option value="all">All</option>
              <option value="running">Running</option>
              <option value="stopped">Stopped</option>
              <option value="error">Error</option>
            </select>
            <select
              bind:value={sortBy}
              class="text-xs bg-surface-2 border border-border rounded-md px-2 py-1 text-text-secondary"
            >
              <option value="name">Name</option>
              <option value="status">Status</option>
              <option value="cpu">CPU</option>
            </select>
          </div>
        </div>

        {#if apps.length === 0}
          <div class="bg-surface-2 border border-border rounded-lg p-8 text-center">
            <p class="text-text-secondary text-sm">No apps deployed yet.</p>
          </div>
        {:else}
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            {#each filteredApps() as app}
              <AppCard {app} metrics={appMetricsMap[app.Slug]} />
            {/each}
          </div>
        {/if}
      </div>

      <!-- Side Panels (2/5) -->
      <div class="xl:col-span-2 flex flex-col gap-4">
        <!-- Active Alerts -->
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Active Alerts</h3>
            <a href="#/alerts" class="text-xs text-accent hover:underline">View all</a>
          </div>
          {#if activeAlerts.length === 0}
            <p class="text-xs text-text-secondary">No active alerts</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each activeAlerts.slice(0, 5) as alert}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full bg-danger shrink-0"></span>
                  <span class="text-text-primary">Rule #{alert.rule_id}</span>
                  <span class="text-text-muted ml-auto">{formatTime(alert.fired_at)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Recent Backups -->
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Recent Backups</h3>
            <a href="#/backups" class="text-xs text-accent hover:underline">View all</a>
          </div>
          {#if recentBackups().length === 0}
            <p class="text-xs text-text-secondary">No backup runs</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each recentBackups() as run}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full shrink-0 {run.status === 'completed' ? 'bg-success' : run.status === 'failed' ? 'bg-danger' : 'bg-warning'}"></span>
                  <span class="text-text-primary truncate">{run.slug}</span>
                  <Badge variant={run.status === 'completed' ? 'success' : 'danger'}>{run.status}</Badge>
                  <span class="text-text-muted ml-auto whitespace-nowrap">{formatTime(run.started_at)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Alert History (recent) -->
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Alert History</h3>
          </div>
          {#if (alertHistory || []).length === 0}
            <p class="text-xs text-text-secondary">No alerts fired</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each (alertHistory || []).slice(0, 5) as h}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full shrink-0 {h.resolved_at ? 'bg-success' : 'bg-danger'}"></span>
                  <span class="text-text-primary">Rule #{h.rule_id}</span>
                  <span class="text-text-muted ml-auto">{formatDate(h.fired_at)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Charts -->
    <div class="mb-3 flex items-center justify-between">
      <h2 class="text-base font-semibold text-text-primary">System Trends</h2>
      <div class="flex gap-1">
        {#each Object.keys(rangeMs) as range}
          <button
            onclick={() => { timeRange = range; loadDashboard() }}
            class="px-2 py-1 text-xs rounded-md border transition-colors
              {timeRange === range ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
          >
            {range}
          </button>
        {/each}
      </div>
    </div>
    {#if cpuHistory.length > 0}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <MetricsChart data={cpuHistory} label="CPU Usage" color="#58a6ff" unit="%" />
        <MetricsChart data={memHistory} label="Memory Usage" color="#3fb950" unit="%" />
      </div>
    {:else}
      <div class="bg-surface-2 border border-border rounded-lg p-8 text-center">
        <p class="text-text-secondary text-sm">No metrics data available.</p>
      </div>
    {/if}
  {/if}
</Layout>
