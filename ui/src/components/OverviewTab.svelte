<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import StatCard from './StatCard.svelte'
  import Badge from './Badge.svelte'

  let { slug, app, services = [], onSwitchTab } = $props()

  // --- State ---
  let requestData = $state(null)
  let metricsData = $state(null)
  let events = $state([])
  let loading = $state(true)

  onMount(async () => {
    const [reqRes, metRes, evtRes] = await Promise.all([
      api.appRequests(slug, '1h'),
      api.appMetrics(slug, '1h'),
      api.getDeployEvents(slug),
    ])
    requestData = reqRes.data
    metricsData = metRes.data
    events = (evtRes.data || []).slice(0, 5)
    loading = false
  })

  // --- Request derived values ---
  let totalRequests = $derived.by(() => {
    const pts = requestData?.points ?? []
    return pts.reduce((s, p) => s + (p.n || 0), 0)
  })

  let totalErrors = $derived.by(() => {
    const pts = requestData?.points ?? []
    return pts.reduce((s, p) => s + (p.e || 0), 0)
  })

  let errorRate = $derived(totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0)

  let avgLatency = $derived.by(() => {
    const pts = (requestData?.points ?? []).filter(p => p.n > 0)
    if (!pts.length) return 0
    return pts.reduce((s, p) => s + (p.al || 0), 0) / pts.length
  })

  let requestSparkline = $derived((requestData?.points ?? []).map(p => p.n || 0))
  let errorSparkline = $derived((requestData?.points ?? []).map(p => p.n > 0 ? (p.e / p.n) * 100 : 0))
  let latencySparkline = $derived((requestData?.points ?? []).map(p => p.al || 0))

  // --- Metrics derived values ---
  function containerTimeSeries(key) {
    const containers = metricsData?.containers ?? {}
    const ids = Object.keys(containers)
    if (!ids.length) return []
    const len = containers[ids[0]]?.points?.length ?? 0
    const result = []
    for (let i = 0; i < len; i++) {
      let sum = 0, count = 0
      for (const id of ids) {
        const pt = containers[id]?.points?.[i]
        if (pt != null) { sum += pt[key] || 0; count++ }
      }
      result.push(count > 0 ? sum / count : 0)
    }
    return result
  }

  let cpuSparkline = $derived(containerTimeSeries('c'))
  let latestCpu = $derived.by(() => {
    const s = cpuSparkline
    return s.length ? s[s.length - 1] : 0
  })

  let memSparkline = $derived.by(() => {
    const containers = metricsData?.containers ?? {}
    const ids = Object.keys(containers)
    if (!ids.length) return []
    const len = containers[ids[0]]?.points?.length ?? 0
    const result = []
    for (let i = 0; i < len; i++) {
      let sumM = 0, sumMl = 0
      for (const id of ids) {
        const pt = containers[id]?.points?.[i]
        if (pt != null) { sumM += pt.m || 0; sumMl += pt.ml || 0 }
      }
      result.push(sumMl > 0 ? (sumM / sumMl) * 100 : 0)
    }
    return result
  })

  let latestMem = $derived.by(() => {
    const containers = metricsData?.containers ?? {}
    const ids = Object.keys(containers)
    if (!ids.length) return { pct: 0, usedMB: 0, totalMB: 0 }
    let sumM = 0, sumMl = 0
    for (const id of ids) {
      const pts = containers[id]?.points ?? []
      const pt = pts[pts.length - 1]
      if (pt) { sumM += pt.m || 0; sumMl += pt.ml || 0 }
    }
    const pct = sumMl > 0 ? (sumM / sumMl) * 100 : 0
    return { pct, usedMB: Math.round(sumM / (1024 * 1024)), totalMB: Math.round(sumMl / (1024 * 1024)) }
  })

  // --- Helpers ---
  function fmtRequests(n) {
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
    return String(n)
  }

  function fmtLatency(ms) {
    if (!ms) return '0ms'
    if (ms >= 1000) return (ms / 1000).toFixed(2) + 's'
    return Math.round(ms) + 'ms'
  }

  function relativeTime(isoStr) {
    if (!isoStr) return ''
    const diff = Date.now() - new Date(isoStr).getTime()
    const secs = Math.floor(diff / 1000)
    if (secs < 60) return 'just now'
    const mins = Math.floor(secs / 60)
    if (mins < 60) return `${mins}m ago`
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) return `${hrs}h ago`
    const days = Math.floor(hrs / 24)
    return `${days}d ago`
  }

  function serviceVariant(state) {
    if (state === 'running') return 'success'
    if (state === 'exited' || state === 'dead') return 'danger'
    if (state === 'restarting') return 'warning'
    return 'warning'
  }

  function eventActionLabel(action) {
    if (action === 'deploy_unstable') return 'deployed (unstable)'
    if (action === 'restart_unstable') return 'restarted (unstable)'
    if (action === 'pull_unstable') return 'pulled (unstable)'
    return action
  }

  function healthVariant(health) {
    if (!health) return 'info'
    if (health === 'healthy') return 'success'
    if (health === 'unhealthy') return 'danger'
    return 'info'
  }

  function eventVariant(action) {
    if (['deploy', 'restart', 'pull'].includes(action)) return 'success'
    if (action?.endsWith('_failed')) return 'danger'
    if (action?.endsWith('_unstable')) return 'warning'
    if (action === 'rollback') return 'warning'
    return 'info'
  }
</script>

<!-- Stat Cards -->
<div class="grid grid-cols-2 lg:grid-cols-5 gap-4 mb-6">
  <StatCard
    label="Requests (1h)"
    value={fmtRequests(totalRequests)}
    sparkline={requestSparkline}
    sparklineColor="#3b82f6"
    onclick={() => onSwitchTab('metrics')}
  />
  <StatCard
    label="Error Rate"
    value="{errorRate.toFixed(1)}%"
    color={errorRate > 5 ? 'text-danger' : 'text-success'}
    sparkline={errorSparkline}
    sparklineColor="#ef4444"
    onclick={() => onSwitchTab('metrics')}
  />
  <StatCard
    label="Avg Latency"
    value={fmtLatency(avgLatency)}
    sparkline={latencySparkline}
    sparklineColor="#a78bfa"
    onclick={() => onSwitchTab('metrics')}
  />
  <StatCard
    label="CPU"
    value="{latestCpu.toFixed(1)}%"
    sparkline={cpuSparkline}
    sparklineColor="#f59e0b"
    onclick={() => onSwitchTab('metrics')}
  />
  <StatCard
    label="Memory"
    value="{latestMem.pct.toFixed(1)}%"
    sub={latestMem.totalMB > 0 ? `${latestMem.usedMB} MB / ${latestMem.totalMB} MB` : ''}
    sparkline={memSparkline}
    sparklineColor="#22c55e"
    onclick={() => onSwitchTab('metrics')}
  />
</div>

<!-- Services -->
{#if services.length > 0}
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
    <h3 class="text-sm font-medium text-text-primary mb-4">Services</h3>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
      {#each services as svc}
        <div class="flex items-center gap-2 px-3 py-2 bg-surface-1 rounded-lg border border-border/30">
          <span class="font-mono text-sm text-text-primary flex-1 truncate">{svc.service}</span>
          <Badge variant={serviceVariant(svc.state)}>{svc.state}</Badge>
          {#if svc.health}
            <Badge variant={healthVariant(svc.health)}>{svc.health}</Badge>
          {/if}
        </div>
      {/each}
    </div>
  </div>
{/if}

<!-- Recent Deployments -->
<div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
  <h3 class="text-sm font-medium text-text-primary mb-4">Recent Deployments</h3>
  {#if loading}
    <div class="space-y-2">
      {#each [1, 2, 3] as _}
        <div class="h-9 bg-surface-1 rounded-lg animate-pulse"></div>
      {/each}
    </div>
  {:else if events.length === 0}
    <p class="text-sm text-text-muted">No recent deployments.</p>
  {:else}
    <div class="space-y-2">
      {#each events as evt}
        <div class="flex items-center gap-3 px-3 py-2 bg-surface-1 rounded-lg border border-border/30 text-sm">
          <Badge variant={eventVariant(evt.action)}>{eventActionLabel(evt.action)}</Badge>
          <span class="text-text-secondary flex-1 truncate">{evt.detail?.split('\n')[0] || '-'}</span>
          <span class="text-xs text-text-muted shrink-0">{relativeTime(evt.created_at)}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>
