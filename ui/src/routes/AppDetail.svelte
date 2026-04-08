<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import LogViewer from '../components/LogViewer.svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'

  let { params } = $props()
  let slug = $derived(params.slug)

  let app = $state(null)
  let activeTab = $state('overview')
  let metricsRange = $state('1h')
  let cpuData = $state([])
  let memData = $state([])
  let requestStats = $state(null)
  let loading = $state(true)

  const tabs = ['overview', 'metrics', 'logs', 'requests']

  const rangeMs = { '1h': 3600000, '6h': 21600000, '24h': 86400000, '7d': 604800000 }

  const statusColors = {
    running: '#3fb950',
    stopped: '#8b949e',
    error: '#f85149'
  }

  onMount(loadApp)

  async function loadApp() {
    try {
      app = await api.getApp(slug)
    } catch {
      push('/')
    } finally {
      loading = false
    }
  }

  async function loadMetrics() {
    const now = new Date().toISOString()
    const from = new Date(Date.now() - rangeMs[metricsRange]).toISOString()
    try {
      const data = await api.appMetrics(slug, from, now)
      cpuData = (data || []).map(m => ({ x: new Date(m.timestamp), y: m.cpu_pct }))
      memData = (data || []).map(m => ({
        x: new Date(m.timestamp),
        y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0
      }))
    } catch { /* ignore */ }
  }

  async function loadRequests() {
    const now = new Date().toISOString()
    const from = new Date(Date.now() - 3600000).toISOString()
    try {
      requestStats = await api.appRequests(slug, from, now)
    } catch { /* ignore */ }
  }

  async function handleRemove() {
    if (!confirm(`Remove ${app.Name}?`)) return
    await api.removeApp(slug)
    push('/')
  }

  $effect(() => {
    if (activeTab === 'metrics') loadMetrics()
    if (activeTab === 'requests') loadRequests()
  })
</script>

<Layout>
  {#if loading}
    <div class="loading">Loading...</div>
  {:else if app}
    <div class="header">
      <a href="#/" class="back">&#8592; Apps</a>
      <div class="title-row">
        <span class="status-dot" style="background: {statusColors[app.Status] || '#8b949e'}"></span>
        <h1>{app.Name}</h1>
        <span class="status-badge" style="color: {statusColors[app.Status] || '#8b949e'}">{app.Status}</span>
      </div>
      {#if app.Domain}
        <a href="https://{app.Domain}" target="_blank" rel="noopener" class="domain">{app.Domain}</a>
      {/if}
    </div>

    <div class="tab-bar">
      {#each tabs as tab}
        <button class="tab" class:active={activeTab === tab} onclick={() => activeTab = tab}>
          {tab}
        </button>
      {/each}
    </div>

    <div class="tab-content">
      {#if activeTab === 'overview'}
        <div class="section">
          <h3>Details</h3>
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-label">Slug</span>
              <span class="detail-value">{app.Slug}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">Status</span>
              <span class="detail-value">{app.Status}</span>
            </div>
            {#if app.Domain}
              <div class="detail-item">
                <span class="detail-label">Domain</span>
                <span class="detail-value">{app.Domain}</span>
              </div>
            {/if}
            {#if app.ComposeFile}
              <div class="detail-item">
                <span class="detail-label">Compose File</span>
                <span class="detail-value mono">{app.ComposeFile}</span>
              </div>
            {/if}
          </div>
        </div>

        {#if app.Labels && Object.keys(app.Labels).length > 0}
          <div class="section">
            <h3>Labels</h3>
            <div class="labels">
              {#each Object.entries(app.Labels) as [key, val]}
                <div class="label-row">
                  <span class="label-key">{key}</span>
                  <span class="label-val">{val}</span>
                </div>
              {/each}
            </div>
          </div>
        {/if}

        <div class="section danger-zone">
          <h3>Danger Zone</h3>
          <button class="btn-danger" onclick={handleRemove}>Remove App</button>
        </div>

      {:else if activeTab === 'metrics'}
        <div class="range-selector">
          {#each Object.keys(rangeMs) as range}
            <button class="range-btn" class:active={metricsRange === range} onclick={() => metricsRange = range}>
              {range}
            </button>
          {/each}
        </div>
        <div class="charts-row">
          <MetricsChart data={cpuData} label="CPU Usage" color="#58a6ff" unit="%" />
          <MetricsChart data={memData} label="Memory Usage" color="#3fb950" unit="%" />
        </div>

      {:else if activeTab === 'logs'}
        <LogViewer {slug} />

      {:else if activeTab === 'requests'}
        {#if requestStats}
          <div class="stats-row">
            <div class="stat-card">
              <span class="stat-label">Total Requests</span>
              <span class="stat-value">{requestStats.total ?? 0}</span>
            </div>
            <div class="stat-card">
              <span class="stat-label">Avg Latency</span>
              <span class="stat-value">{requestStats.avg_latency_ms?.toFixed(1) ?? '0'}ms</span>
            </div>
            <div class="stat-card">
              <span class="stat-label">Error Rate</span>
              <span class="stat-value">{requestStats.error_rate?.toFixed(1) ?? '0'}%</span>
            </div>
          </div>
        {:else}
          <p class="empty">No request data available.</p>
        {/if}
      {/if}
    </div>
  {/if}
</Layout>

<style>
  .loading { color: #8b949e; padding: 2rem; text-align: center; }

  .header { margin-bottom: 1.25rem; }
  .back {
    color: #58a6ff; text-decoration: none; font-size: 0.8rem;
    display: inline-block; margin-bottom: 0.5rem;
  }
  .back:hover { text-decoration: underline; }
  .title-row { display: flex; align-items: center; gap: 0.5rem; }
  .status-dot { width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0; }
  h1 { margin: 0; font-size: 1.3rem; font-weight: 600; color: #e1e4e8; }
  .status-badge {
    font-size: 0.75rem; text-transform: capitalize;
    padding: 0.15rem 0.5rem; border: 1px solid currentColor;
    border-radius: 12px;
  }
  .domain {
    display: inline-block; margin-top: 0.35rem;
    color: #58a6ff; font-size: 0.8rem; text-decoration: none;
  }
  .domain:hover { text-decoration: underline; }

  .tab-bar {
    display: flex; gap: 0; border-bottom: 1px solid #21262d;
    margin-bottom: 1.25rem;
  }
  .tab {
    padding: 0.5rem 1rem; background: none; border: none;
    color: #8b949e; cursor: pointer; font-size: 0.85rem;
    text-transform: capitalize; border-bottom: 2px solid transparent;
    transition: color 0.15s;
  }
  .tab:hover { color: #e1e4e8; }
  .tab.active { color: #e1e4e8; border-bottom-color: #58a6ff; }

  .section {
    background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px;
    padding: 1rem; margin-bottom: 1rem;
  }
  .section h3 {
    margin: 0 0 0.75rem; font-size: 0.9rem; font-weight: 600; color: #e1e4e8;
  }
  .detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
  .detail-item { display: flex; flex-direction: column; gap: 0.2rem; }
  .detail-label { font-size: 0.72rem; color: #8b949e; text-transform: uppercase; letter-spacing: 0.04em; }
  .detail-value { font-size: 0.85rem; color: #e1e4e8; }
  .detail-value.mono { font-family: 'SF Mono', monospace; font-size: 0.8rem; }

  .labels { display: flex; flex-direction: column; gap: 0.35rem; }
  .label-row {
    display: flex; gap: 0.5rem; font-size: 0.8rem;
    padding: 0.3rem 0.5rem; background: #161b22; border-radius: 4px;
  }
  .label-key { color: #8b949e; min-width: 200px; word-break: break-all; }
  .label-val { color: #e1e4e8; word-break: break-all; }

  .danger-zone { border-color: #f8514933; }
  .danger-zone h3 { color: #f85149; }
  .btn-danger {
    padding: 0.4rem 0.8rem; background: none; border: 1px solid #f85149;
    border-radius: 4px; color: #f85149; cursor: pointer; font-size: 0.8rem;
  }
  .btn-danger:hover { background: #f8514920; }

  .range-selector {
    display: flex; gap: 0.35rem; margin-bottom: 1rem;
  }
  .range-btn {
    padding: 0.3rem 0.6rem; background: #21262d; border: 1px solid #30363d;
    border-radius: 4px; color: #8b949e; cursor: pointer; font-size: 0.75rem;
  }
  .range-btn.active { color: #58a6ff; border-color: #58a6ff; }

  .charts-row {
    display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;
  }

  .stats-row {
    display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.75rem;
  }
  .stat-card {
    background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px;
    padding: 1rem; display: flex; flex-direction: column; gap: 0.25rem;
  }
  .stat-label {
    font-size: 0.75rem; color: #8b949e; text-transform: uppercase; letter-spacing: 0.04em;
  }
  .stat-value { font-size: 1.5rem; font-weight: 600; color: #e1e4e8; }
  .empty { color: #8b949e; font-size: 0.85rem; }
</style>
