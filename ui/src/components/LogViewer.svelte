<script>
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'

  let { slug, service = '' } = $props()

  let lines = $state([])
  let ws = $state(null)
  let following = $state(true)
  let container
  let services = $state([])
  let selectedService = $state(service)
  let showTimestamps = $state(true)

  onMount(async () => {
    const { data } = await api.getAppServices(slug)
    if (data) {
      services = data.map(s => s.service).sort()
      if (!selectedService && services.length > 0) {
        selectedService = services[0]
      }
    }
    connect()
  })
  onDestroy(() => { if (ws) ws.close() })

  function connect() {
    if (ws) ws.close()
    lines = []
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    let url = `${proto}//${window.location.host}/api/apps/${slug}/logs?follow=true&tail=200`
    if (selectedService) url += `&service=${selectedService}`

    ws = new WebSocket(url)
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      if (msg.error) {
        lines = [{ stream: 'stderr', line: msg.error, ts: '' }]
        return
      }
      lines = [...lines.slice(-999), msg]
      if (following && container) {
        requestAnimationFrame(() => {
          container.scrollTop = container.scrollHeight
        })
      }
    }
    ws.onclose = () => { ws = null }
  }

  function switchService(svc) {
    selectedService = svc
    connect()
  }

  function toggleFollow() {
    following = !following
    if (following && container) {
      container.scrollTop = container.scrollHeight
    }
  }

  function clear() { lines = [] }

  function downloadLogs() {
    const text = lines.map((l) => `${l.ts || ''} [${l.stream}] ${l.line}`).join('\n')
    const blob = new Blob([text], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${slug}-${selectedService || 'all'}-logs.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const btnBase = 'px-2.5 py-1 text-xs font-medium rounded-md transition-colors'
  const btnInactive = 'text-text-secondary hover:text-text-primary hover:bg-surface-3/50'
</script>

<div class="flex flex-col h-[600px] rounded-xl overflow-hidden border border-border/50 shadow-sm">
  <div class="flex items-center gap-1.5 px-4 py-2.5 bg-surface-2 border-b border-border/50 flex-wrap">
    {#if services.length > 1}
      <div class="flex items-center gap-1 bg-surface-3/40 rounded-lg p-0.5">
        {#each services as svc}
          <button
            onclick={() => switchService(svc)}
            class="px-2.5 py-1 text-xs font-medium rounded-md transition-colors
              {selectedService === svc ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
          >
            {svc}
          </button>
        {/each}
      </div>
      <div class="w-px h-4 bg-border/50 mx-1"></div>
    {/if}
    <div class="flex items-center gap-1 bg-surface-3/40 rounded-lg p-0.5">
      <button onclick={toggleFollow}
        class="{btnBase} {following ? 'bg-surface-2 text-success shadow-sm' : btnInactive}">
        <span class="inline-flex items-center gap-1">
          {#if following}<span class="w-1.5 h-1.5 rounded-full bg-success"></span>{/if}
          {following ? 'Following' : 'Paused'}
        </span>
      </button>
      <button onclick={clear} class="{btnBase} {btnInactive}">Clear</button>
      <button onclick={() => showTimestamps = !showTimestamps}
        class="{btnBase} {showTimestamps ? 'bg-surface-2 text-accent shadow-sm' : btnInactive}">
        Timestamps
      </button>
    </div>
    <button onclick={downloadLogs} class="{btnBase} {btnInactive} ml-1">
      <span class="inline-flex items-center gap-1">
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5M16.5 12L12 16.5m0 0L7.5 12m4.5 4.5V3" /></svg>
        Download
      </span>
    </button>
    <span class="ml-auto text-xs text-text-muted tabular-nums">{lines.length} lines</span>
  </div>

  <div
    bind:this={container}
    class="flex-1 overflow-y-auto bg-[#0c0c0c] light:bg-[#1a1a2e] font-mono text-[13px] leading-5 p-4 selection:bg-accent/30"
  >
    {#if lines.length === 0}
      <div class="flex items-center justify-center h-full text-[#555] text-sm">Waiting for logs...</div>
    {:else}
      {#each lines as line}
        <div class="whitespace-pre-wrap break-all py-px hover:bg-white/[0.03] {line.stream === 'stderr' ? 'text-red-400' : 'text-[#d4d4d4] light:text-[#c8c8d8]'}">
          {#if showTimestamps && line.ts}<span class="text-[#555] mr-3 select-none">{line.ts}</span>{/if}<span>{line.line}</span>
        </div>
      {/each}
    {/if}
  </div>
</div>
