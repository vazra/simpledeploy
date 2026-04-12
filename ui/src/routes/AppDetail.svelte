<script>
  import { onMount, onDestroy } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import LogViewer from '../components/LogViewer.svelte'
  import ConfigTab from '../components/ConfigTab.svelte'
  import EventsTab from '../components/EventsTab.svelte'
  import EnvEditor from '../components/EnvEditor.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Badge from '../components/Badge.svelte'
  import Button from '../components/Button.svelte'
  import Modal from '../components/Modal.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import { toasts } from '../lib/stores/toast.js'
  import { connection } from '../lib/stores/connection.svelte.js'

  let { params } = $props()
  let slug = $derived(params.slug)

  let app = $state(null)
  let activeTab = $state('overview')
  let metricsRange = $state('1h')
  let cpuDatasets = $state([])
  let memDatasets = $state([])
  let memSummary = $state('')
  let memRaw = $state([])
  let netRxDatasets = $state([])
  let netTxDatasets = $state([])
  let diskReadDatasets = $state([])
  let diskWriteDatasets = $state([])
  let requestStats = $state(null)
  let loading = $state(true)
  let showDeleteModal = $state(false)
  let removing = $state(false)
  let backupConfigs = $state([])
  let backupRuns = $state([])
  let services = $state([])
  let restoreTarget = $state(null)
  let showBackupForm = $state(false)
  let showRestartModal = $state(false)
  let showScaleModal = $state(false)
  let actionLoading = $state('')
  let scaleInputs = $state({})
  let editDomain = $state('')
  let editAccessAllow = $state('')

  // Backup form
  let bStrategy = $state('postgres')
  let bTarget = $state('s3')
  let bCron = $state('0 2 * * *')
  let bRetention = $state(7)

  let metricsInterval = $state(10)
  let containerIds = $state([])  // ['Total', 'abc123...', 'def456...']
  let visibleContainers = $state(new Set())

  const tabs = ['overview', 'logs', 'events', 'metrics', 'backups', 'config', 'environment']
  const ranges = ['1h', '6h', '24h', '1w', '1m', '1yr']

  let pollTimer = null
  function startPolling() {
    stopPolling()
    pollTimer = setInterval(async () => {
      const res = await api.getApp(slug)
      if (res.error) return
      app = res.data
      editDomain = app?.Domain || ''
      editAccessAllow = app?.Labels?.['simpledeploy.access.allow'] || ''
      if (!app.deploying) {
        stopPolling()
        loadServices()
        if (app.Status === 'error') activeTab = 'events'
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
  onMount(loadApp)
  onDestroy(() => { unsubReconnect(); stopPolling() })

  async function loadApp() {
    const [appRes] = await Promise.all([
      api.getApp(slug),
      loadRequests(),
      loadServices(),
    ])
    if (appRes.error) { push('/'); return }
    app = appRes.data
    editDomain = app?.Domain || ''
    editAccessAllow = app?.Labels?.['simpledeploy.access.allow'] || ''
    loading = false
    if (app?.deploying) startPolling()
  }

  function formatBytes(bytes) {
    if (!bytes) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
  }

  const containerColors = ['#60a5fa', '#34d399', '#fbbf24', '#a78bfa', '#fb923c', '#f87171', '#2dd4bf', '#e879f9']

  function buildContainerDatasets(containers, extract, baseColor) {
    const ids = Object.keys(containers).filter(id => id !== '')
    const single = ids.length <= 1

    // extract(p) returns either a number (y value) or {y, extra} for metadata
    function toPoint(p, extracted) {
      if (extracted == null) return { x: new Date(p.t * 1000), y: null }
      if (typeof extracted === 'object') return { x: new Date(p.t * 1000), y: extracted.y, extra: extracted.extra }
      return { x: new Date(p.t * 1000), y: extracted }
    }

    // Per-container series
    const perContainer = ids.map((id, i) => {
      const pts = containers[id]?.points || []
      return {
        label: id,
        color: single ? baseColor : containerColors[i % containerColors.length],
        data: pts.map(p => toPoint(p, extract(p))),
      }
    })

    if (single) return perContainer

    // Combined "Total" line: sum y and extra values at each timestamp
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
    const res = await api.appMetrics(slug, metricsRange)
    const data = res.data
    if (!data?.containers) return

    const interval = data.interval || 10
    metricsInterval = interval
    const c = data.containers

    // Track container IDs for visibility toggles
    const ids = Object.keys(c).filter(id => id !== '')
    const hasMultiple = ids.length > 1
    const labels = hasMultiple ? ['Total', ...ids.map(id => id)] : ids.map(id => id)
    containerIds = labels
    // Preserve existing visibility; default all visible
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

    // mem summary: sum bytes across containers at latest timestamp
    const latestTs = Math.max(...ids.flatMap(id => (c[id]?.points || []).map(p => p.t)))
    let totalMem = 0, totalLimit = 0
    for (const id of ids) {
      const pts = c[id]?.points || []
      const latest = pts.filter(p => p.t === latestTs && p.c != null).pop()
      if (latest) { totalMem += latest.m || 0; totalLimit = Math.max(totalLimit, latest.ml || 0) }
    }
    memSummary = totalLimit ? `${formatBytes(totalMem)} / ${formatBytes(totalLimit)}` : ''
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

  async function loadServices() {
    const res = await api.getAppServices(slug)
    if (res.error) return
    services = res.data || []
  }

  async function loadRequests() {
    const res = await api.appRequests(slug, metricsRange)
    if (res.error) return
    requestStats = res.data
  }

  async function loadBackups() {
    const [cRes, rRes] = await Promise.all([
      api.listBackupConfigs(slug),
      api.listBackupRuns(slug),
    ])
    backupConfigs = cRes.data || []
    backupRuns = rRes.data || []
  }

  async function handleRemove() {
    removing = true
    const res = await api.removeApp(slug)
    removing = false
    showDeleteModal = false
    if (!res.error) push('/')
  }

  async function createBackupConfig() {
    const res = await api.createBackupConfig(slug, {
      strategy: bStrategy, target: bTarget,
      cron_expr: bCron, retention_days: bRetention,
    })
    if (!res.error) { showBackupForm = false; loadBackups() }
  }

  async function deleteBackupConfig(id) {
    await api.deleteBackupConfig(id)
    loadBackups()
  }

  async function triggerBackup() {
    await api.triggerBackup(slug)
    loadBackups()
  }

  async function confirmRestore() {
    if (!restoreTarget) return
    await api.restore(restoreTarget)
    restoreTarget = null
    loadBackups()
  }

  async function handleRestart() {
    actionLoading = 'restart'
    const res = await api.restartApp(slug)
    actionLoading = ''
    showRestartModal = false
    if (!res.error) {
      app = { ...app, deploying: true }
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
      activeTab = 'events'
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

  async function saveDomain() {
    const { error } = await api.updateDomain(slug, editDomain)
    if (!error) await loadApp()
  }

  async function saveAccessAllow() {
    const { error } = await api.updateAccess(slug, editAccessAllow)
    if (!error) await loadApp()
  }

  $effect(() => {
    if (activeTab === 'metrics') loadMetrics()
    if (activeTab === 'backups') loadBackups()
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
        Dashboard
      </a>
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <span class="w-3 h-3 rounded-full ring-2 ring-surface-0 {app.Status === 'running' ? 'bg-success' : app.Status === 'degraded' ? 'bg-warning' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
          <h1 class="text-2xl font-semibold text-text-primary tracking-tight">{app.Name}</h1>
          <Badge variant={app.Status === 'running' ? 'success' : app.Status === 'degraded' ? 'warning' : app.Status === 'error' ? 'danger' : 'default'}>{app.Status}</Badge>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          {#if app.Status === 'running' || app.Status === 'degraded'}
            <Button variant="secondary" size="sm" onclick={() => showRestartModal = true} loading={actionLoading === 'restart'}>Restart</Button>
            <Button variant="secondary" size="sm" onclick={handleStop} loading={actionLoading === 'stop'}>Stop</Button>
          {:else if app.Status === 'stopped'}
            <Button variant="primary" size="sm" onclick={handleStart} loading={actionLoading === 'start'}>Start</Button>
          {/if}
          <Button variant="secondary" size="sm" onclick={handlePull} loading={actionLoading === 'pull'}>Pull &amp; Update</Button>
          <Button variant="secondary" size="sm" onclick={() => { scaleInputs = {}; showScaleModal = true }}>Scale</Button>
          {#if app?.deploying}
            <button
              onclick={cancelDeploy}
              class="px-3 py-1.5 text-xs rounded-lg bg-danger text-white hover:bg-danger/90 transition-colors"
            >
              Cancel Deploy
            </button>
          {/if}
          <Button variant="danger" size="sm" onclick={() => showDeleteModal = true}>Delete</Button>
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
      <!-- Stats -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <StatCard label="Total Requests" value={requestStats?.total ?? 0} />
        <StatCard label="Avg Latency" value="{requestStats?.avg_latency_ms?.toFixed(1) ?? '0'}ms" />
        <StatCard label="Error Rate" value="{requestStats?.error_rate?.toFixed(1) ?? '0'}%"
          color={parseFloat(requestStats?.error_rate || 0) > 5 ? 'text-danger' : 'text-success'} />
        <StatCard label="Status" value={app.Status} color={app.Status === 'running' ? 'text-success' : app.Status === 'degraded' ? 'text-warning' : 'text-danger'} />
      </div>

      <!-- Details -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
        <h3 class="text-sm font-medium text-text-primary mb-4">Details</h3>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 text-sm">
          <div>
            <span class="text-xs text-text-muted font-medium">Slug</span>
            <p class="text-text-primary mt-0.5">{app.Slug}</p>
          </div>
          <div>
            <span class="text-xs text-text-muted font-medium">Status</span>
            <p class="text-text-primary mt-0.5 capitalize">{app.Status}</p>
          </div>
          <div>
            <span class="text-xs text-text-muted font-medium">Domain</span>
            <div class="flex items-center gap-2 mt-1">
              <input
                bind:value={editDomain}
                placeholder="example.com"
                class="px-2 py-1 text-sm bg-surface-0 border border-border/50 rounded-lg font-mono focus:outline-none focus:border-accent w-64"
              />
              {#if editDomain !== (app?.Domain || '')}
                <button
                  onclick={saveDomain}
                  class="px-2 py-1 text-xs rounded bg-accent text-white hover:bg-accent/90 transition-colors"
                >
                  Save
                </button>
              {/if}
            </div>
          </div>
          <div>
            <span class="text-xs text-text-muted font-medium">IP Allowlist</span>
            <div class="flex items-center gap-2 mt-1">
              <input
                bind:value={editAccessAllow}
                placeholder="e.g. 10.0.0.0/8, 192.168.1.5"
                class="px-2 py-1 text-sm bg-surface-0 border border-border/50 rounded-lg font-mono focus:outline-none focus:border-accent w-80"
              />
              {#if editAccessAllow !== (app?.Labels?.['simpledeploy.access.allow'] || '')}
                <button
                  onclick={saveAccessAllow}
                  class="px-2 py-1 text-xs rounded bg-accent text-white hover:bg-accent/90 transition-colors"
                >
                  Save
                </button>
              {/if}
            </div>
            <p class="text-xs text-text-muted mt-1">
              {#if editAccessAllow}
                Only these IPs/CIDRs can access this app
              {:else}
                All traffic allowed (no restriction)
              {/if}
            </p>
          </div>
          {#if app.ComposeFile}
            <div>
              <span class="text-xs text-text-muted font-medium">Compose File</span>
              <p class="text-text-primary mt-0.5 font-mono text-xs">{app.ComposeFile}</p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Labels -->
      {#if app.Labels && Object.keys(app.Labels).length > 0}
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
          <h3 class="text-sm font-medium text-text-primary mb-4">Labels</h3>
          <div class="flex flex-col gap-1">
            {#each Object.entries(app.Labels) as [key, val]}
              <div class="flex gap-2 text-xs px-2 py-1.5 bg-surface-1 rounded-lg">
                <span class="text-text-secondary min-w-48 break-all">{key}</span>
                <span class="text-text-primary break-all">{val}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if services.length > 0}
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
          <h3 class="text-sm font-medium text-text-primary mb-4">Services</h3>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
            {#each services as svc}
              <div class="flex items-center justify-between bg-surface-1 rounded-lg px-4 py-3">
                <span class="text-sm text-text-primary font-mono">{svc.service}</span>
                <div class="flex items-center gap-2">
                  <Badge variant={svc.state === 'running' ? 'success' : svc.state === 'exited' ? 'danger' : 'warning'}>{svc.state}</Badge>
                  {#if svc.health}
                    <Badge variant={svc.health === 'healthy' ? 'success' : svc.health === 'unhealthy' ? 'danger' : 'info'}>{svc.health}</Badge>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

    {:else if activeTab === 'logs'}
      <LogViewer {slug} />

    {:else if activeTab === 'events'}
      <EventsTab {slug} deploying={app?.deploying} />

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
      <!-- Backup Configs -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-medium text-text-primary">Backup Configs</h3>
          <div class="flex gap-2">
            <Button size="sm" onclick={triggerBackup}>Run Now</Button>
            <Button size="sm" variant="secondary" onclick={() => showBackupForm = !showBackupForm}>
              {showBackupForm ? 'Cancel' : 'New Config'}
            </Button>
          </div>
        </div>

        {#if showBackupForm}
          <form onsubmit={(e) => { e.preventDefault(); createBackupConfig() }} class="bg-surface-1 rounded-md p-4 mb-4 grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs text-text-muted mb-2">Strategy</label>
              <select bind:value={bStrategy} class="w-full px-2 py-1.5 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
                <option>postgres</option><option>volume</option>
              </select>
            </div>
            <div>
              <label class="block text-xs text-text-muted mb-2">Target</label>
              <select bind:value={bTarget} class="w-full px-2 py-1.5 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
                <option>s3</option><option>local</option>
              </select>
            </div>
            <div>
              <label class="block text-xs text-text-muted mb-2">Cron Schedule</label>
              <input bind:value={bCron} class="w-full px-2 py-1.5 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary" />
            </div>
            <div>
              <label class="block text-xs text-text-muted mb-2">Retention (days)</label>
              <input type="number" bind:value={bRetention} class="w-full px-2 py-1.5 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary" />
            </div>
            <div class="col-span-2 flex justify-end">
              <Button type="submit" size="sm">Create</Button>
            </div>
          </form>
        {/if}

        {#if backupConfigs.length === 0}
          <p class="text-xs text-text-secondary">No backup configs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Strategy</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Target</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Schedule</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Retention</th>
                <th class="py-3 px-4"></th>
              </tr></thead>
              <tbody class="divide-y divide-border/30">
                {#each backupConfigs as c}
                  <tr class="hover:bg-surface-hover">
                    <td class="py-3 px-4">{c.strategy}</td>
                    <td class="py-3 px-4">{c.target}</td>
                    <td class="py-3 px-4 font-mono text-xs">{c.cron_expr}</td>
                    <td class="py-3 px-4">{c.retention_days}d</td>
                    <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => deleteBackupConfig(c.id)}>Delete</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

      <!-- Backup Runs -->
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h3 class="text-sm font-medium text-text-primary mb-4">Backup Runs</h3>
        {#if backupRuns.length === 0}
          <p class="text-xs text-text-secondary">No backup runs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">ID</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Status</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Started</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Finished</th>
                <th class="py-3 px-4"></th>
              </tr></thead>
              <tbody class="divide-y divide-border/30">
                {#each backupRuns as r}
                  <tr class="hover:bg-surface-hover">
                    <td class="py-3 px-4">{r.id}</td>
                    <td class="py-3 px-4"><Badge variant={r.status === 'completed' ? 'success' : 'danger'}>{r.status}</Badge></td>
                    <td class="py-3 px-4">{r.started_at ? new Date(r.started_at).toLocaleString() : '-'}</td>
                    <td class="py-3 px-4">{r.finished_at ? new Date(r.finished_at).toLocaleString() : '-'}</td>
                    <td class="py-3 px-4"><Button variant="secondary" size="sm" onclick={() => restoreTarget = r.id}>Restore</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {:else if activeTab === 'config'}
      <ConfigTab {slug} />
    {:else if activeTab === 'environment'}
      <EnvEditor {slug} />
    {/if}

    {#if showDeleteModal}
      <Modal title="Delete App" message="This will remove {app.Name} and all its data. Are you sure?" onConfirm={handleRemove} onCancel={() => showDeleteModal = false} />
    {/if}

    {#if restoreTarget}
      <Modal title="Confirm Restore" message="This will restore the backup. Are you sure?" onConfirm={confirmRestore} onCancel={() => restoreTarget = null} />
    {/if}

    {#if showRestartModal}
      <Modal title="Restart App" message="This will force-recreate all containers for {app.Name}. Continue?" onConfirm={handleRestart} onCancel={() => showRestartModal = false} />
    {/if}

    {#if showScaleModal}
      <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
        <button class="absolute inset-0 bg-black/60 backdrop-blur-sm" onclick={() => showScaleModal = false} aria-label="Close"></button>
        <div class="relative bg-surface-2 border border-border/50 rounded-2xl p-6 min-w-80 max-w-md shadow-2xl animate-scale-in">
          <h3 class="text-lg font-semibold tracking-tight text-text-primary mb-4">Scale Services</h3>
          <div class="space-y-3 mb-5">
            {#each app.Services || ['web'] as svc}
              {@const name = typeof svc === 'string' ? svc : svc.Name || svc}
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
