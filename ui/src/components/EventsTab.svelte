<script>
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'
  import Badge from './Badge.svelte'
  import Skeleton from './Skeleton.svelte'

  let { slug, deploying = false } = $props()

  let events = $state([])
  let loading = $state(true)
  let expandedEvents = $state(new Set())

  // Live terminal state
  let liveLines = $state([])
  let liveStatus = $state(null) // null = active, OutputLine with done=true
  let ws = null
  let terminalEl

  onMount(loadEvents)
  onDestroy(disconnect)

  async function loadEvents() {
    const res = await api.getDeployEvents(slug)
    events = res.data || []
    loading = false
  }

  let connected = false

  $effect(() => {
    if (deploying && !connected) {
      connected = true
      connect()
    }
    if (!deploying) {
      connected = false
    }
  })

  function connect() {
    disconnect()
    liveLines = []
    liveStatus = null
    const socket = api.deployLogsWs(slug)
    ws = socket
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      if (msg.done) {
        liveStatus = msg
        ws.close()
        ws = null
        setTimeout(() => {
          loadEvents()
        }, 1500)
        return
      }
      liveLines = [...liveLines.slice(-499), msg]
      if (terminalEl) {
        requestAnimationFrame(() => {
          terminalEl.scrollTop = terminalEl.scrollHeight
        })
      }
    }
    ws.onclose = () => {
      if (!liveStatus) {
        liveStatus = { done: true, action: 'disconnected' }
      }
    }
  }

  function disconnect() {
    if (ws) {
      ws.close()
      ws = null
    }
  }
</script>

{#if deploying || liveLines.length > 0}
  <div class="mb-4">
    <div class="flex items-center gap-2 px-4 py-2.5 bg-surface-2 border border-border/50 border-b-0 rounded-t-xl">
      {#if !liveStatus}
        <span class="w-2 h-2 rounded-full bg-success animate-pulse"></span>
        <span class="text-xs font-medium text-text-primary">Deploying...</span>
      {:else}
        <span class="w-2 h-2 rounded-full {liveStatus.action?.includes('failed') ? 'bg-danger' : 'bg-success'}"></span>
        <span class="text-xs font-medium text-text-primary">
          {liveStatus.action?.includes('failed') ? 'Failed' : 'Complete'}
        </span>
        <Badge variant={liveStatus.action?.includes('failed') ? 'danger' : 'success'}>{liveStatus.action}</Badge>
      {/if}
    </div>
    <div
      bind:this={terminalEl}
      class="bg-[#0c0c0c] light:bg-[#1a1a2e] border border-border/50 border-t-0 rounded-b-xl font-mono text-[13px] leading-5 p-4 overflow-y-auto max-h-80 selection:bg-accent/30"
    >
      {#each liveLines as line}
        <div class="whitespace-pre-wrap break-all py-px {line.stream === 'stderr' ? 'text-red-400' : 'text-[#d4d4d4]'}">
          {line.line}
        </div>
      {/each}
      {#if liveLines.length === 0 && !liveStatus}
        <div class="text-[#555] text-sm">Waiting for output...</div>
      {/if}
    </div>
  </div>
{/if}

{#if loading}
  <Skeleton type="card" count={3} />
{:else if events.length === 0 && !deploying && liveLines.length === 0}
  <p class="text-sm text-text-muted">No deploy events yet.</p>
{:else}
  <div class="space-y-2">
    {#each events as evt, i}
      {@const variant = ['deploy', 'restart', 'pull'].includes(evt.action) ? 'success'
        : ['deploy_failed', 'restart_failed', 'pull_failed'].includes(evt.action) ? 'danger'
        : evt.action === 'rollback' ? 'warning'
        : 'info'}
      {@const hasDetail = !!evt.detail}
      {@const expanded = expandedEvents.has(i)}
      <div>
        <button
          class="flex items-center gap-3 text-sm px-3 py-2 bg-surface-1 border border-border/30 rounded-lg w-full text-left {hasDetail ? 'cursor-pointer hover:bg-surface-3' : 'cursor-default'}"
          onclick={() => {
            if (!hasDetail) return
            const next = new Set(expandedEvents)
            if (next.has(i)) next.delete(i)
            else next.add(i)
            expandedEvents = next
          }}
        >
          {#if hasDetail}
            <span class="text-text-muted text-xs transition-transform {expanded ? 'rotate-90' : ''}">&gt;</span>
          {/if}
          <Badge {variant}>{evt.action}</Badge>
          <span class="text-text-secondary flex-1 truncate">{hasDetail ? evt.detail.split('\n')[0] : '-'}</span>
          <span class="text-xs text-text-muted shrink-0">{evt.created_at ? new Date(evt.created_at).toLocaleString() : ''}</span>
        </button>
        {#if hasDetail && expanded}
          <div class="bg-surface-0 rounded-lg p-3 mt-1 border border-border/30">
            <pre class="text-xs font-mono whitespace-pre-wrap overflow-x-auto max-h-80 overflow-y-auto text-text-secondary">{evt.detail}</pre>
          </div>
        {/if}
      </div>
    {/each}
  </div>
{/if}
