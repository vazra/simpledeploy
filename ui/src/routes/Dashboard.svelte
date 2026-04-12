<script>
  import { onMount, onDestroy } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import AppCard from '../components/AppCard.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import Badge from '../components/Badge.svelte'
  import Button from '../components/Button.svelte'
  import DeployWizard from '../components/DeployWizard.svelte'
  import { api } from '../lib/api.js'
  import { connection } from '../lib/stores/connection.svelte.js'

  let apps = $state([])
  let cpuHistory = $state([])
  let memHistory = $state([])
  let memRawHistory = $state([])
  let loading = $state(true)
  let loadError = $state(false)
  let latestMetrics = $state(null)
  let alertHistory = $state([])
  let alertRules = $state([])
  let backupRunsByApp = $state({})
  let appMetricsMap = $state({})
  let hostMemory = $state(0)
  let appRequestsMap = $state({})
  let timeRange = $state('1h')
  let metricsInterval = $state(10)

  const ranges = ['1h', '6h', '24h', '1w', '1m', '1yr']

  let filterStatus = $state('all')
  let sortBy = $state('name')
  let searchQuery = $state('')

  // Deploy form
  let showDeployPanel = $state(false)

  const unsubReconnect = connection.onReconnect(() => loadDashboard())
  onMount(loadDashboard)
  onDestroy(unsubReconnect)

  async function loadDashboard() {
    loading = true
    loadError = false

    const [appsRes, metricsRes, histRes, rulesRes, dockerRes] = await Promise.all([
      api.listApps(),
      api.systemMetrics(timeRange),
      api.alertHistory(),
      api.listAlertRules(),
      api.dockerInfo(),
    ])
    if (dockerRes.data?.memory) hostMemory = dockerRes.data.memory

    if (appsRes.error) {
      loading = false
      loadError = true
      return
    }

    apps = appsRes.data || []
    alertHistory = histRes.data || []
    alertRules = rulesRes.data || []

    const metricsData = metricsRes.data
    if (metricsData?.containers) {
      const sysPoints = metricsData.containers['']?.points || []
      const interval = metricsData.interval || 10

      if (sysPoints.length > 0) {
        const latest = sysPoints[sysPoints.length - 1]
        latestMetrics = {
          cpu: (latest.c ?? 0).toFixed(1),
          memUsed: formatBytes(latest.m || 0),
          memTotal: formatBytes(latest.ml || 0),
          memPct: latest.ml ? ((latest.m / latest.ml) * 100).toFixed(1) : '0',
          netRx: formatBytes(latest.nr || 0),
          netTx: formatBytes(latest.nt || 0),
          diskRead: formatBytes(latest.dr || 0),
          diskWrite: formatBytes(latest.dw || 0),
        }
        cpuHistory = sysPoints.map(p => ({ x: new Date(p.t * 1000), y: p.c ?? null }))
        memHistory = sysPoints.map(p => ({
          x: new Date(p.t * 1000),
          y: p.ml ? ((p.m || 0) / p.ml) * 100 : null,
        }))
        memRawHistory = sysPoints.map(p => ({ bytes: p.m || 0, limit: p.ml || 0 }))
        metricsInterval = interval
      }
    }

    loading = false
    loadPerAppData()
  }

  async function loadPerAppData() {
    await Promise.all(
      apps.map(async (app) => {
        const slug = app.Slug || app.slug
        const [mRes, rRes, bRes] = await Promise.all([
          api.appMetrics(slug, '1h'),
          api.appRequests(slug, '1h'),
          api.listBackupRuns(slug),
        ])
        if (mRes.data?.containers) {
          const containers = Object.values(mRes.data.containers)
          // Sum latest point from each container
          let totalCpu = 0, totalMem = 0, totalMemLimit = 0
          let hasData = false
          for (const ctr of containers) {
            const latest = (ctr.points || []).filter(p => p.c != null).pop()
            if (latest) {
              hasData = true
              totalCpu += latest.c || 0
              totalMem += latest.m || 0
              totalMemLimit += latest.ml || 0
            }
          }
          if (hasData) {
            const effectiveLimit = hostMemory ? Math.min(totalMemLimit, hostMemory) : totalMemLimit
            appMetricsMap = { ...appMetricsMap, [slug]: {
              cpu: totalCpu,
              memPct: effectiveLimit ? (totalMem / effectiveLimit) * 100 : 0,
              memBytes: totalMem,
              memLimit: effectiveLimit,
            }}
          }
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

  const metricNames = { cpu_pct: 'CPU', mem_pct: 'Memory %', mem_bytes: 'Memory' }

  function ruleLabel(ruleId) {
    const r = alertRules.find(r => r.id === ruleId)
    if (!r) return `Rule #${ruleId}`
    const app = r.app_slug || 'All apps'
    return `${metricNames[r.metric] || r.metric} - ${app}`
  }

  function ruleCondition(ruleId) {
    const r = alertRules.find(r => r.id === ruleId)
    if (!r) return ''
    return `${r.operator} ${alertFormatValue(r.metric, r.threshold)}`
  }

  function alertFormatValue(metric, value) {
    if (value == null) return '-'
    if (metric === 'mem_bytes') {
      if (value >= 1 << 30) return `${(value / (1 << 30)).toFixed(1)} GB`
      if (value >= 1 << 20) return `${(value / (1 << 20)).toFixed(1)} MB`
      return `${value.toFixed(0)} B`
    }
    if (metric === 'cpu_pct' || metric === 'mem_pct') return `${value.toFixed(1)}%`
    return value.toFixed(1)
  }

  function alertTriggerValue(h) {
    const r = alertRules.find(r => r.id === h.rule_id)
    return r ? alertFormatValue(r.metric, h.value) : h.value?.toFixed(1) ?? '-'
  }

  function timeAgo(ts) {
    if (!ts) return '-'
    const diff = Math.floor((Date.now() - new Date(ts).getTime()) / 1000)
    if (diff < 60) return 'just now'
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
    return `${Math.floor(diff / 86400)}d ago`
  }

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

        <!-- App Summary -->
        <div class="flex gap-2 mb-4">
          <button onclick={() => filterStatus = 'all'} class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-surface-2 border border-border/50 text-sm cursor-pointer transition-colors hover:border-border {filterStatus === 'all' ? 'border-accent/50' : ''}">
            <span class="text-text-muted">Total</span>
            <span class="font-semibold text-text-primary">{apps.length}</span>
          </button>
          <button onclick={() => filterStatus = filterStatus === 'running' ? 'all' : 'running'} class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-surface-2 border border-border/50 text-sm cursor-pointer transition-colors hover:border-border {filterStatus === 'running' ? 'border-accent/50' : ''}">
            <span class="text-text-muted">Running</span>
            <span class="font-semibold text-success">{runningCount}</span>
          </button>
          <button onclick={() => filterStatus = filterStatus === 'stopped' ? 'all' : 'stopped'} class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-surface-2 border border-border/50 text-sm cursor-pointer transition-colors hover:border-border {filterStatus === 'stopped' ? 'border-accent/50' : ''}">
            <span class="text-text-muted">Stopped</span>
            <span class="font-semibold {stoppedCount > 0 ? 'text-danger' : 'text-text-secondary'}">{stoppedCount}</span>
          </button>
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
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border {activeAlerts.length > 0 ? 'border-danger/30' : 'border-border/50'}">
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-2">
              <h3 class="text-sm font-semibold text-text-primary">Active Alerts</h3>
              {#if activeAlerts.length > 0}
                <span class="text-xs font-medium text-danger bg-danger/10 px-1.5 py-0.5 rounded-full">{activeAlerts.length}</span>
              {/if}
            </div>
            <a href="#/alerts" class="text-xs text-accent hover:underline">View all</a>
          </div>
          {#if activeAlerts.length === 0}
            <div class="flex items-center gap-2 text-xs text-text-secondary">
              <svg class="w-4 h-4 text-success" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
              All clear, no active alerts
            </div>
          {:else}
            <div class="flex flex-col gap-2.5">
              {#each activeAlerts.slice(0, 5) as alert}
                <a href="#/alerts" class="flex flex-col gap-1 rounded-lg bg-danger/5 border border-danger/10 px-3 py-2 hover:bg-danger/10 transition-colors">
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-medium text-text-primary">{ruleLabel(alert.rule_id)}</span>
                    <span class="text-xs text-text-muted">{timeAgo(alert.fired_at)}</span>
                  </div>
                  <div class="flex items-center gap-2 text-xs text-text-secondary">
                    <span>Triggered at {alertTriggerValue(alert)}</span>
                    <span class="text-text-muted">{ruleCondition(alert.rule_id)}</span>
                  </div>
                </a>
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
            <h3 class="text-sm font-semibold text-text-primary">Recent Alerts</h3>
            <a href="#/alerts" class="text-xs text-accent hover:underline">History</a>
          </div>
          {#if (alertHistory || []).length === 0}
            <p class="text-xs text-text-secondary">No alerts fired yet</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each (alertHistory || []).slice(0, 5) as h}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full shrink-0 {h.resolved_at ? 'bg-success' : 'bg-danger'}"></span>
                  <span class="text-text-primary truncate" title={ruleLabel(h.rule_id)}>{ruleLabel(h.rule_id)}</span>
                  <Badge variant={h.resolved_at ? 'success' : 'danger'}>{h.resolved_at ? 'Resolved' : 'Active'}</Badge>
                  <span class="text-text-muted ml-auto whitespace-nowrap" title={formatDate(h.fired_at)}>{timeAgo(h.fired_at)}</span>
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
        {#each ranges as range}
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
        <MetricsChart data={cpuHistory} label="CPU Usage" color="#3b82f6" unit="%" interval={metricsInterval} />
        <MetricsChart data={memHistory} label="Memory Usage" color="#22c55e" unit="%" subtitle="{latestMetrics?.memUsed || '0'} / {latestMetrics?.memTotal || '0'}" interval={metricsInterval}
          tooltipFormat={(i, pct) => {
            const r = memRawHistory[i]
            return r ? `${pct.toFixed(1)}% (${formatBytes(r.bytes)} / ${formatBytes(r.limit)})` : `${pct.toFixed(1)}%`
          }} />
      </div>
    {:else}
      <div class="bg-surface-2 rounded-xl p-12 shadow-sm border border-border/50 text-center">
        <p class="text-text-secondary text-sm">No metrics data available.</p>
      </div>
    {/if}
    </div>
  {/if}

  <DeployWizard open={showDeployPanel} onclose={() => showDeployPanel = false} onComplete={() => { showDeployPanel = false; loadDashboard() }} />
</Layout>
