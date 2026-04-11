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
  let cpuData = $state([])
  let memData = $state([])
  let netRxData = $state([])
  let netTxData = $state([])
  let diskReadData = $state([])
  let diskWriteData = $state([])
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

  const tabs = ['overview', 'logs', 'events', 'metrics', 'backups', 'config', 'environment']
  const rangeMs = { '1h': 3600000, '6h': 21600000, '24h': 86400000, '7d': 604800000 }

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

  // gapThreshold: if two points are further apart than this, break the line.
  // Based on the coarsest tier expected for each range.
  // Between normal spacing and smallest gap: 5m-tier=5min, 1h-tier=1hr
  // A missing point doubles the spacing, so threshold sits at 1.5x normal
  const gapThreshold = { '1h': 450000, '6h': 450000, '24h': 5400000, '7d': 5400000 }

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

  async function loadMetrics() {
    const now = new Date().toISOString()
    const from = new Date(Date.now() - rangeMs[metricsRange]).toISOString()
    const res = await api.appMetrics(slug, from, now)
    const data = res.data || []
    cpuData = withGaps(metricsRange, data.map((m) => ({ x: new Date(m.timestamp), y: m.cpu_pct })))
    memData = withGaps(metricsRange, data.map((m) => ({ x: new Date(m.timestamp), y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0 })))
    netRxData = withGaps(metricsRange, data.map((m) => ({ x: new Date(m.timestamp), y: m.net_rx || 0 })))
    netTxData = withGaps(metricsRange, data.map((m) => ({ x: new Date(m.timestamp), y: m.net_tx || 0 })))
    diskReadData = withGaps(metricsRange, data.map((m) => ({ x: new Date(m.timestamp), y: m.disk_read || 0 })))
    diskWriteData = withGaps(metricsRange, data.map((m) => ({ x: new Date(m.timestamp), y: m.disk_write || 0 })))
  }

  async function loadServices() {
    const res = await api.getAppServices(slug)
    if (res.error) return
    services = res.data || []
  }

  async function loadRequests() {
    const now = new Date().toISOString()
    const from = new Date(Date.now() - 3600000).toISOString()
    const res = await api.appRequests(slug, from, now)
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
          <span class="w-3 h-3 rounded-full ring-2 ring-surface-0 {app.Status === 'running' ? 'bg-success' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
          <h1 class="text-2xl font-semibold text-text-primary tracking-tight">{app.Name}</h1>
          <Badge variant={app.Status === 'running' ? 'success' : app.Status === 'error' ? 'danger' : 'default'}>{app.Status}</Badge>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          {#if app.Status === 'running'}
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
        <StatCard label="Status" value={app.Status} color={app.Status === 'running' ? 'text-success' : 'text-danger'} />
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
      <div class="flex gap-1 mb-4">
        {#each Object.keys(rangeMs) as range}
          <button
            onclick={() => { metricsRange = range; loadMetrics() }}
            class="px-2 py-1 text-xs rounded-md border transition-colors
              {metricsRange === range ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
          >
            {range}
          </button>
        {/each}
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <MetricsChart data={cpuData} label="CPU Usage" color="#3b82f6" unit="%" />
        <MetricsChart data={memData} label="Memory Usage" color="#22c55e" unit="%" />
        <MetricsChart data={netRxData} label="Network RX" color="#eab308" unit=" B/s" />
        <MetricsChart data={netTxData} label="Network TX" color="#a78bfa" unit=" B/s" />
        <MetricsChart data={diskReadData} label="Disk Read" color="#fb923c" unit=" B/s" />
        <MetricsChart data={diskWriteData} label="Disk Write" color="#ef4444" unit=" B/s" />
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
