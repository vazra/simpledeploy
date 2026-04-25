<script>
  import { onMount, onDestroy } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import LogViewer from '../components/LogViewer.svelte'
  import OverviewTab from '../components/OverviewTab.svelte'
  import EventsTab from '../components/EventsTab.svelte'
  import ActivityTab from '../components/ActivityTab.svelte'
  import SettingsTab from '../components/SettingsTab.svelte'
  import BackupsTab from '../components/BackupsTab.svelte'
  import ActionModal from '../components/ActionModal.svelte'
  import Badge from '../components/Badge.svelte'
  import Button from '../components/Button.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import { connection } from '../lib/stores/connection.svelte.js'
  import { formatBytes } from '../lib/format.js'

  let { params } = $props()
  let slug = $derived(params.slug)

  let app = $state(null)
  let activeTab = $state('overview')
  let loading = $state(true)
  let services = $state([])

  // Metrics state
  let metricsRange = $state('1h')
  let cpuDatasets = $state([])
  let memDatasets = $state([])
  let memSummary = $state('')
  let netRxDatasets = $state([])
  let netTxDatasets = $state([])
  let diskReadDatasets = $state([])
  let diskWriteDatasets = $state([])
  let requestsPerSecDatasets = $state([])
  let errorRateDatasets = $state([])
  let latencyDatasets = $state([])
  let metricsInterval = $state(10)
  let containerIds = $state([])
  let visibleContainers = $state(new Set())

  // Action modal state
  let actionModal = $state({ show: false, action: '' })
  let showMoreMenu = $state(false)
  let showScaleModal = $state(false)
  let showRestartModal = $state(false)
  let actionLoading = $state('')
  let scaleInputs = $state({})

  const tabs = ['overview', 'events', 'logs', 'activity', 'metrics', 'backups', 'settings']
  const ranges = ['1h', '6h', '24h', '1w', '1m', '1yr']

  // Filter non-simpledeploy labels for display as tags
  let displayLabels = $derived.by(() => {
    if (!app?.Labels) return []
    return Object.entries(app.Labels)
      .filter(([k]) => !k.startsWith('simpledeploy.'))
      .map(([k, v]) => v || k)
  })

  // --- Polling for deploy status ---
  let pollTimer = null
  function startPolling() {
    stopPolling()
    pollTimer = setInterval(async () => {
      const res = await api.getApp(slug)
      if (res.error) return
      app = res.data
      if (!app.deploying) {
        stopPolling()
        loadServices()
      }
    }, 3000)
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  const unsubReconnect = connection.onReconnect(() => loadApp())
  onMount(() => {
    loadApp()
    const hash = window.location.hash
    const tabMatch = hash.match(/[?&]tab=(\w+)/)
    if (tabMatch && tabs.includes(tabMatch[1])) {
      activeTab = tabMatch[1]
    }
  })
  onDestroy(() => { unsubReconnect(); stopPolling() })

  async function loadApp() {
    const [appRes] = await Promise.all([
      api.getApp(slug),
      loadServices(),
    ])
    if (appRes.error) { push('/'); return }
    app = appRes.data
    loading = false
    if (app?.deploying) startPolling()
  }

  async function loadServices() {
    const res = await api.getAppServices(slug)
    if (res.error) return
    services = res.data || []
  }

  // --- Action handlers ---
  async function handleRestart() {
    actionLoading = 'restart'
    const res = await api.restartApp(slug)
    actionLoading = ''
    showRestartModal = false
    if (!res.error) {
      app = { ...app, deploying: true }
      actionModal = { show: true, action: 'Restarting' }
      startPolling()
    }
  }

  async function handleStop() {
    actionLoading = 'stop'
    await api.stopApp(slug)
    actionLoading = ''
    loadApp()
  }

  async function handleStart() {
    actionLoading = 'start'
    await api.startApp(slug)
    actionLoading = ''
    loadApp()
  }

  async function handlePull() {
    actionLoading = 'pull'
    const res = await api.pullApp(slug)
    actionLoading = ''
    if (!res.error) {
      app = { ...app, deploying: true }
      actionModal = { show: true, action: 'Pulling & Updating' }
      startPolling()
    }
  }

  async function cancelDeploy() {
    await api.cancelDeploy(slug)
    await loadApp()
  }

  async function handleScale() {
    actionLoading = 'scale'
    const scales = {}
    for (const [svc, n] of Object.entries(scaleInputs)) {
      scales[svc] = parseInt(n) || 1
    }
    await api.scaleApp(slug, scales)
    actionLoading = ''
    showScaleModal = false
    loadApp()
  }

  function closeActionModal() {
    actionModal = { show: false, action: '' }
    loadApp()
  }

  // --- Metrics ---
  const containerColors = ['#60a5fa', '#34d399', '#fbbf24', '#a78bfa', '#fb923c', '#f87171', '#2dd4bf', '#e879f9']

  function buildContainerDatasets(containers, extract, baseColor) {
    const ids = Object.keys(containers).filter(id => id !== '')
    const single = ids.length <= 1

    function toPoint(p, extracted) {
      if (extracted == null) return { x: new Date(p.t * 1000), y: null }
      if (typeof extracted === 'object') return { x: new Date(p.t * 1000), y: extracted.y, extra: extracted.extra }
      return { x: new Date(p.t * 1000), y: extracted }
    }

    const perContainer = ids.map((id, i) => {
      const pts = containers[id]?.points || []
      return {
        label: id,
        color: single ? baseColor : containerColors[i % containerColors.length],
        data: pts.map(p => toPoint(p, extract(p))),
      }
    })

    if (single) return perContainer

    const byTs = new Map()
    for (const id of ids) {
      for (const p of (containers[id]?.points || [])) {
        const raw = extract(p)
        const v = typeof raw === 'object' ? raw?.y : raw
        const e = typeof raw === 'object' ? raw?.extra : undefined
        if (v == null) continue
        const existing = byTs.get(p.t)
        if (existing) {
          existing.y += v
          if (e != null && existing.extra != null) existing.extra += e
        } else {
          byTs.set(p.t, { y: v, extra: e != null ? e : undefined })
        }
      }
    }
    const totalData = [...byTs.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([t, v]) => {
        const pt = { x: new Date(t * 1000), y: v.y }
        if (v.extra != null) pt.extra = v.extra
        return pt
      })

    return [
      { label: 'Total', color: baseColor, data: totalData },
      ...perContainer,
    ]
  }

  async function loadMetrics() {
    const [metRes, reqRes] = await Promise.all([
      api.appMetrics(slug, metricsRange),
      api.appRequests(slug, metricsRange),
    ])

    // Resource metrics
    const data = metRes.data
    if (data?.containers) {
      const interval = data.interval || 10
      metricsInterval = interval
      const c = data.containers

      const ids = Object.keys(c).filter(id => id !== '')
      const hasMultiple = ids.length > 1
      const labels = hasMultiple ? ['Total', ...ids] : [...ids]
      containerIds = labels
      if (visibleContainers.size === 0 || ![...visibleContainers].some(v => labels.includes(v))) {
        visibleContainers = new Set(labels)
      }

      cpuDatasets = buildContainerDatasets(c, p => p.c ?? null, '#3b82f6')
      memDatasets = buildContainerDatasets(c, p => {
        if (!p.ml) return null
        return { y: ((p.m || 0) / p.ml) * 100, extra: p.m || 0 }
      }, '#22c55e')
      netRxDatasets = buildContainerDatasets(c, p => p.nr ?? null, '#eab308')
      netTxDatasets = buildContainerDatasets(c, p => p.nt ?? null, '#a78bfa')
      diskReadDatasets = buildContainerDatasets(c, p => p.dr ?? null, '#fb923c')
      diskWriteDatasets = buildContainerDatasets(c, p => p.dw ?? null, '#ef4444')

      const latestTs = Math.max(...ids.flatMap(id => (c[id]?.points || []).map(p => p.t)))
      let totalMem = 0, totalLimit = 0
      for (const id of ids) {
        const pts = c[id]?.points || []
        const latest = pts.filter(p => p.t === latestTs && p.c != null).pop()
        if (latest) { totalMem += latest.m || 0; totalLimit = Math.max(totalLimit, latest.ml || 0) }
      }
      memSummary = totalLimit ? `${formatBytes(totalMem)} / ${formatBytes(totalLimit)}` : ''
    }

    // Request metrics
    const reqData = reqRes.data
    if (reqData?.points) {
      const points = reqData.points
      const interval = reqData.interval || 10
      requestsPerSecDatasets = [{
        label: 'Requests/s', color: '#3b82f6',
        data: points.map(p => ({ x: new Date(p.t * 1000), y: p.n != null ? p.n / interval : null }))
      }]
      errorRateDatasets = [{
        label: 'Error Rate', color: '#ef4444',
        data: points.map(p => ({ x: new Date(p.t * 1000), y: (p.n != null && p.n > 0) ? (p.e / p.n) * 100 : null }))
      }]
      latencyDatasets = [{
        label: 'Avg Latency', color: '#a78bfa',
        data: points.map(p => ({ x: new Date(p.t * 1000), y: p.al ?? null }))
      }]
    }
  }

  function toggleContainer(label) {
    const next = new Set(visibleContainers)
    if (next.has(label)) next.delete(label)
    else next.add(label)
    visibleContainers = next
  }

  function filterDatasets(ds) {
    return ds.filter(d => visibleContainers.has(d.label))
  }

  $effect(() => {
    if (activeTab === 'metrics') loadMetrics()
  })
</script>

<Layout>
  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" />
      <Skeleton type="card" count={3} />
    </div>
  {:else if app}
    <!-- Header -->
    <div class="mb-8">
      <a href="#/" class="text-sm text-text-muted hover:text-text-primary inline-flex items-center gap-1.5 mb-3 transition-colors">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" /></svg>
        Apps
      </a>
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div class="flex items-center gap-3 flex-wrap">
          <span class="w-3 h-3 rounded-full ring-2 ring-surface-0 shrink-0 {app.Status === 'running' ? 'bg-success' : (app.Status === 'degraded' || app.Status === 'unstable') ? 'bg-warning' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
          <h1 class="text-2xl font-semibold text-text-primary tracking-tight">{app.Name}</h1>
          <Badge variant={app.Status === 'running' ? 'success' : (app.Status === 'degraded' || app.Status === 'unstable') ? 'warning' : app.Status === 'error' ? 'danger' : 'default'}>{app.Status}</Badge>
          {#each displayLabels as label}
            <span class="px-2 py-0.5 text-[11px] rounded-md bg-surface-3/60 text-text-secondary">{label}</span>
          {/each}
        </div>
        <div class="flex flex-wrap items-center gap-2">
          {#if app.Status === 'running' || app.Status === 'degraded' || app.Status === 'unstable'}
            <Button variant="secondary" size="sm" onclick={() => showRestartModal = true} loading={actionLoading === 'restart'}>Restart</Button>
            <Button variant="secondary" size="sm" onclick={handleStop} loading={actionLoading === 'stop'}>Stop</Button>
          {:else if app.Status === 'stopped'}
            <Button variant="primary" size="sm" onclick={handleStart} loading={actionLoading === 'start'}>Start</Button>
          {/if}
          <Button variant="secondary" size="sm" onclick={handlePull} loading={actionLoading === 'pull'}>Pull & Update</Button>
          <!-- More dropdown -->
          <div class="relative">
            <Button variant="ghost" size="sm" onclick={() => showMoreMenu = !showMoreMenu}>
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM12.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM18.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0z" />
              </svg>
            </Button>
            {#if showMoreMenu}
              <button class="fixed inset-0 z-10" onclick={() => showMoreMenu = false} aria-label="Close menu"></button>
              <div class="absolute right-0 top-full mt-1 bg-surface-2 border border-border/50 rounded-lg shadow-xl py-1 min-w-36 z-20">
                <button onclick={() => { scaleInputs = {}; showScaleModal = true; showMoreMenu = false }} class="w-full text-left px-3 py-2 text-sm text-text-secondary hover:text-text-primary hover:bg-surface-hover transition-colors">Scale</button>
                {#if app?.deploying}
                  <button onclick={() => { cancelDeploy(); showMoreMenu = false }} class="w-full text-left px-3 py-2 text-sm text-danger hover:bg-surface-hover transition-colors">Cancel Deploy</button>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      </div>
      {#if app.Domain}
        <a href="https://{app.Domain}" target="_blank" rel="noopener" class="text-sm text-accent hover:underline mt-1 inline-block">{app.Domain}</a>
      {/if}
    </div>

    <!-- Tabs -->
    <div class="flex overflow-x-auto border-b border-border/50 mb-8 -mx-4 px-4 md:mx-0 md:px-0">
      {#each tabs as tab}
        <button
          onclick={() => activeTab = tab}
          class="px-4 py-3 text-sm capitalize font-medium transition-colors border-b-2 whitespace-nowrap shrink-0
            {activeTab === tab ? 'text-text-primary border-accent' : 'text-text-muted border-transparent hover:text-text-primary'}"
        >
          {tab}
        </button>
      {/each}
    </div>

    <!-- Tab Content -->
    {#if activeTab === 'overview'}
      <OverviewTab {slug} {app} {services} onSwitchTab={(tab) => activeTab = tab} />

    {:else if activeTab === 'events'}
      <EventsTab {slug} deploying={app?.deploying || false} />

    {:else if activeTab === 'logs'}
      <LogViewer {slug} />

    {:else if activeTab === 'activity'}
      <ActivityTab {slug} />

    {:else if activeTab === 'metrics'}
      <div class="flex flex-wrap items-center gap-3 mb-4">
        <div class="flex gap-1">
          {#each ranges as range}
            <button
              onclick={() => { metricsRange = range; loadMetrics() }}
              class="px-2 py-1 text-xs rounded-md border transition-colors
                {metricsRange === range ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
            >
              {range}
            </button>
          {/each}
        </div>
        {#if containerIds.length > 1}
          <div class="flex flex-wrap items-center gap-2 ml-auto">
            {#each containerIds as cid, i}
              <label class="flex items-center gap-1.5 text-xs cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={visibleContainers.has(cid)}
                  onchange={() => toggleContainer(cid)}
                  class="w-3 h-3 rounded accent-current"
                  style="color: {cid === 'Total' ? '#94a3b8' : containerColors[i - 1] || containerColors[(i - 1) % containerColors.length]}"
                />
                <span class="text-text-secondary font-mono">{cid}</span>
              </label>
            {/each}
          </div>
        {/if}
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <!-- Request metrics -->
        <MetricsChart datasets={requestsPerSecDatasets} label="Requests / sec" unit="" interval={metricsInterval} />
        <MetricsChart datasets={errorRateDatasets} label="Error Rate" unit="%" interval={metricsInterval} />
        <MetricsChart datasets={latencyDatasets} label="Avg Latency" unit="ms" interval={metricsInterval} />
        <!-- Resource metrics -->
        <MetricsChart datasets={filterDatasets(cpuDatasets)} label="CPU Usage" unit="%" interval={metricsInterval} />
        <MetricsChart datasets={filterDatasets(memDatasets)} label="Memory Usage" unit="%" subtitle={memSummary} interval={metricsInterval} formatValue={formatBytes} />
        <MetricsChart datasets={filterDatasets(netRxDatasets)} label="Network RX" unit=" B/s" interval={metricsInterval}
          tooltipFormat={(i, v) => `${formatBytes(v)}/s`} />
        <MetricsChart datasets={filterDatasets(netTxDatasets)} label="Network TX" unit=" B/s" interval={metricsInterval}
          tooltipFormat={(i, v) => `${formatBytes(v)}/s`} />
        <MetricsChart datasets={filterDatasets(diskReadDatasets)} label="Disk Read" unit=" B/s" interval={metricsInterval}
          tooltipFormat={(i, v) => `${formatBytes(v)}/s`} />
        <MetricsChart datasets={filterDatasets(diskWriteDatasets)} label="Disk Write" unit=" B/s" interval={metricsInterval}
          tooltipFormat={(i, v) => `${formatBytes(v)}/s`} />
      </div>

    {:else if activeTab === 'backups'}
      <BackupsTab {slug} />

    {:else if activeTab === 'settings'}
      <SettingsTab {slug} {app} {services} onAppUpdated={loadApp} />
    {/if}

    <!-- Action Modal -->
    <ActionModal {slug} action={actionModal.action} show={actionModal.show} onclose={closeActionModal} />

    <!-- Restart confirmation -->
    {#if showRestartModal}
      <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
        <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={() => showRestartModal = false} aria-label="Close"></button>
        <div class="relative bg-surface-2 border border-border/50 rounded-2xl p-6 min-w-80 max-w-md shadow-2xl animate-scale-in">
          <h3 class="text-lg font-semibold text-text-primary tracking-tight mb-2">Restart App</h3>
          <p class="text-sm text-text-secondary mb-5">This will force-recreate all containers for {app.Name}. Continue?</p>
          <div class="flex justify-end gap-2">
            <button onclick={() => showRestartModal = false} class="px-4 py-2 text-sm border border-border/50 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors">Cancel</button>
            <Button onclick={handleRestart} loading={actionLoading === 'restart'}>Restart</Button>
          </div>
        </div>
      </div>
    {/if}

    <!-- Scale modal -->
    {#if showScaleModal}
      <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
        <button class="absolute inset-0 bg-black/60 backdrop-blur-sm" onclick={() => showScaleModal = false} aria-label="Close"></button>
        <div class="relative bg-surface-2 border border-border/50 rounded-2xl p-6 min-w-80 max-w-md shadow-2xl animate-scale-in">
          <h3 class="text-lg font-semibold tracking-tight text-text-primary mb-4">Scale Services</h3>
          <div class="space-y-3 mb-5">
            {#each services.length ? services : [{ service: 'web' }] as svc}
              {@const name = svc.service || 'web'}
              <div class="flex items-center gap-3">
                <label class="text-sm text-text-secondary w-24">{name}</label>
                <input
                  type="number"
                  min="0"
                  value={scaleInputs[name] ?? 1}
                  oninput={(e) => scaleInputs = {...scaleInputs, [name]: e.currentTarget.value}}
                  class="w-20 px-2.5 py-1.5 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary"
                />
              </div>
            {/each}
          </div>
          <div class="flex justify-end gap-2">
            <button onclick={() => showScaleModal = false} class="px-3 py-1.5 text-sm border border-border/50 rounded-lg text-text-secondary hover:text-text-primary transition-colors">Cancel</button>
            <Button onclick={handleScale} loading={actionLoading === 'scale'}>Apply</Button>
          </div>
        </div>
      </div>
    {/if}
  {/if}
</Layout>
