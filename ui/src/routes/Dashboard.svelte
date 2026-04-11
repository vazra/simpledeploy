<script>
  import { onMount, onDestroy } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import AppCard from '../components/AppCard.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import Badge from '../components/Badge.svelte'
  import Button from '../components/Button.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import { api } from '../lib/api.js'
  import { connection } from '../lib/stores/connection.svelte.js'

  let apps = $state([])
  let cpuHistory = $state([])
  let memHistory = $state([])
  let loading = $state(true)
  let loadError = $state(false)
  let latestMetrics = $state(null)
  let alertHistory = $state([])
  let backupRunsByApp = $state({})
  let appMetricsMap = $state({})
  let appRequestsMap = $state({})
  let timeRange = $state('1h')

  const rangeMs = { '1h': 3600000, '6h': 21600000, '24h': 86400000, '7d': 604800000 }

  let filterStatus = $state('all')
  let sortBy = $state('name')
  let searchQuery = $state('')

  // Deploy form
  let showDeployPanel = $state(false)
  let deployName = $state('')
  let deployCompose = $state('')
  let deployInputMode = $state('paste')
  let deploying = $state(false)

  async function handleDeploy() {
    if (!deployName.trim() || !deployCompose.trim()) return
    deploying = true
    const encoded = btoa(deployCompose)
    const res = await api.deploy(deployName.trim(), encoded)
    deploying = false
    if (!res.error) {
      showDeployPanel = false
      deployName = ''
      deployCompose = ''
      loadDashboard()
    }
  }

  function handleFileUpload(e) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => { deployCompose = reader.result }
    reader.readAsText(file)
  }

  const unsubReconnect = connection.onReconnect(() => loadDashboard())
  onMount(loadDashboard)
  onDestroy(unsubReconnect)

  const gapThreshold = { '1h': 120000, '6h': 600000, '24h': 600000, '7d': 7200000 }

  function withGaps(range, points) {
    if (points.length < 2) return points
    const threshold = gapThreshold[range] || 600000
    const result = [points[0]]
    for (let i = 1; i < points.length; i++) {
      const gap = points[i].x - points[i - 1].x
      if (gap > threshold) {
        result.push({ x: new Date(points[i - 1].x.getTime() + 1), y: null })
      }
      result.push(points[i])
    }
    return result
  }

  async function loadDashboard() {
    loading = true
    loadError = false
    const now = new Date().toISOString()
    const from = new Date(Date.now() - rangeMs[timeRange]).toISOString()

    const [appsRes, metricsRes, histRes] = await Promise.all([
      api.listApps(),
      api.systemMetrics(from, now),
      api.alertHistory(),
    ])

    if (appsRes.error) {
      loading = false
      loadError = true
      return
    }

    apps = appsRes.data || []
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
      cpuHistory = withGaps(timeRange, metricsData.map((m) => ({ x: new Date(m.timestamp), y: m.cpu_pct })))
      memHistory = withGaps(timeRange, metricsData.map((m) => ({
        x: new Date(m.timestamp),
        y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0,
      })))
    }

    loading = false
    loadPerAppData(now)
  }

  async function loadPerAppData(now) {
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
          appMetricsMap = { ...appMetricsMap, [slug]: {
            cpu: latest.cpu_pct,
            memPct: latest.mem_limit ? (latest.mem_bytes / latest.mem_limit) * 100 : 0,
          }}
        }
        if (rRes.data) {
          appRequestsMap = { ...appRequestsMap, [slug]: rRes.data }
        }
        if (bRes.data && bRes.data.length > 0) {
          backupRunsByApp = { ...backupRunsByApp, [slug]: bRes.data }
        }
      })
    )
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

  let recentBackups = $derived.by(() => {
    const all = []
    for (const [slug, runs] of Object.entries(backupRunsByApp)) {
      for (const run of runs.slice(0, 3)) {
        all.push({ ...run, slug })
      }
    }
    return all.sort((a, b) => new Date(b.started_at) - new Date(a.started_at)).slice(0, 5)
  })

  let filteredApps = $derived.by(() => {
    let result = apps
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase()
      result = result.filter((a) => (a.Name || '').toLowerCase().includes(q) || (a.Slug || '').toLowerCase().includes(q))
    }
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
  {#if loadError}
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="w-12 h-12 rounded-full bg-red-500/10 flex items-center justify-center mb-4">
        <svg class="w-6 h-6 text-danger" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
        </svg>
      </div>
      <p class="text-text-secondary text-sm mb-3">Unable to connect to backend</p>
      <Button size="sm" variant="secondary" onclick={loadDashboard}>Retry</Button>
    </div>
  {:else if loading}
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      {#each Array(4) as _}<Skeleton type="card" />{/each}
    </div>
    <div class="grid grid-cols-3 gap-3 mb-4">
      {#each Array(3) as _}<Skeleton type="card" />{/each}
    </div>
    <div class="grid grid-cols-2 gap-4">
      {#each Array(2) as _}<Skeleton type="chart" />{/each}
    </div>
  {:else}
    <div class="animate-fade-in-up">
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
    <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 mb-4">
      <StatCard label="Total Apps" value={apps.length} />
      <StatCard label="Running" value={runningCount} color="text-success" />
      <button onclick={() => filterStatus = filterStatus === 'stopped' ? 'all' : 'stopped'} class="text-left">
        <StatCard label="Stopped / Error" value={stoppedCount} color={stoppedCount > 0 ? 'text-danger' : 'text-text-secondary'} />
      </button>
    </div>

    <!-- Main Content: Apps + Sidebar panels -->
    <div class="grid grid-cols-1 lg:grid-cols-5 gap-5 mb-6">
      <!-- Apps Grid (3/5) -->
      <div class="lg:col-span-3">
        <div class="flex flex-wrap items-center gap-2 mb-3">
          <h2 class="text-lg font-semibold text-text-primary tracking-tight flex-1 min-w-0">Applications</h2>
          <div class="flex items-center gap-2">
            <Button size="sm" onclick={() => showDeployPanel = true}>Deploy App</Button>
            <select
              bind:value={filterStatus}
              class="text-xs bg-surface-2 border border-border/50 rounded-lg px-3 py-1.5 text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent/30"
            >
              <option value="all">All</option>
              <option value="running">Running</option>
              <option value="stopped">Stopped</option>
              <option value="error">Error</option>
            </select>
            <select
              bind:value={sortBy}
              class="text-xs bg-surface-2 border border-border/50 rounded-lg px-3 py-1.5 text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent/30"
            >
              <option value="name">Name</option>
              <option value="status">Status</option>
              <option value="cpu">CPU</option>
            </select>
          </div>
        </div>

        <div class="relative mb-4">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          <input
            bind:value={searchQuery}
            placeholder="Search apps..."
            class="w-full pl-10 pr-4 py-2 bg-surface-2 border border-border/50 rounded-lg text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-accent/30"
          />
        </div>

        {#if apps.length === 0}
          <div class="bg-surface-2 rounded-xl p-12 shadow-sm border border-border/50 text-center">
            <div class="w-12 h-12 rounded-xl bg-accent/10 flex items-center justify-center mx-auto mb-4">
              <svg class="w-6 h-6 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
              </svg>
            </div>
            <p class="text-text-primary font-medium mb-1">No apps deployed yet</p>
            <p class="text-sm text-text-muted mb-4">Deploy your first app to get started</p>
            <Button size="sm" onclick={() => showDeployPanel = true}>Deploy App</Button>
          </div>
        {:else}
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            {#each filteredApps as app}
              <AppCard {app} metrics={appMetricsMap[app.Slug]} />
            {/each}
          </div>
        {/if}
      </div>

      <!-- Side Panels (2/5) -->
      <div class="lg:col-span-2 grid grid-cols-1 sm:grid-cols-3 lg:grid-cols-1 gap-4 lg:gap-5">
        <!-- Active Alerts -->
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
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
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Recent Backups</h3>
            <a href="#/backups" class="text-xs text-accent hover:underline">View all</a>
          </div>
          {#if recentBackups.length === 0}
            <p class="text-xs text-text-secondary">No backup runs</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each recentBackups as run}
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
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
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
      <h2 class="text-lg font-semibold text-text-primary tracking-tight">System Trends</h2>
      <div class="flex gap-1">
        {#each Object.keys(rangeMs) as range}
          <button
            onclick={() => { timeRange = range; loadDashboard() }}
            class="px-2 py-1 text-xs rounded-md border transition-colors
              {timeRange === range ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
          >
            {range}
          </button>
        {/each}
      </div>
    </div>
    {#if cpuHistory.length > 0}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <MetricsChart data={cpuHistory} label="CPU Usage" color="#3b82f6" unit="%" />
        <MetricsChart data={memHistory} label="Memory Usage" color="#22c55e" unit="%" />
      </div>
    {:else}
      <div class="bg-surface-2 rounded-xl p-12 shadow-sm border border-border/50 text-center">
        <p class="text-text-secondary text-sm">No metrics data available.</p>
      </div>
    {/if}
    </div>
  {/if}

  <SlidePanel title="Deploy App" open={showDeployPanel} onclose={() => showDeployPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); handleDeploy() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs font-medium text-text-muted mb-2">App Name</label>
        <input
          bind:value={deployName}
          required
          placeholder="my-app"
          class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30"
        />
        <p class="text-xs text-text-muted mt-1">Lowercase letters, numbers, hyphens</p>
      </div>

      <div>
        <label class="block text-xs font-medium text-text-muted mb-2">Compose File</label>
        <div class="flex gap-1 mb-2">
          <button
            type="button"
            onclick={() => deployInputMode = 'paste'}
            class="px-2 py-1 text-xs rounded border transition-colors {deployInputMode === 'paste' ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
          >Paste</button>
          <button
            type="button"
            onclick={() => deployInputMode = 'upload'}
            class="px-2 py-1 text-xs rounded border transition-colors {deployInputMode === 'upload' ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
          >Upload</button>
        </div>

        {#if deployInputMode === 'paste'}
          <textarea
            bind:value={deployCompose}
            required
            rows="12"
            placeholder="version: '3'&#10;services:&#10;  web:&#10;    image: nginx:latest&#10;    ports:&#10;      - '80:80'"
            class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary font-mono focus:outline-none focus:ring-2 focus:ring-accent/30 resize-y"
          ></textarea>
        {:else}
          <input
            type="file"
            accept=".yml,.yaml"
            onchange={handleFileUpload}
            class="w-full text-sm text-text-secondary file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border file:border-border file:text-sm file:bg-surface-3 file:text-text-primary hover:file:bg-surface-3/80"
          />
          {#if deployCompose}
            <p class="text-xs text-success mt-1">File loaded ({deployCompose.length} chars)</p>
          {/if}
        {/if}
      </div>

      <Button type="submit" loading={deploying} disabled={!deployName.trim() || !deployCompose.trim()}>
        Deploy
      </Button>
    </form>
  </SlidePanel>
</Layout>
