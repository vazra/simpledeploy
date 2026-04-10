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
</script>

<div class="flex flex-col h-[500px]">
  <div class="flex items-center gap-2 px-3 py-2 bg-surface-1 border border-border rounded-t-lg flex-wrap">
    {#if services.length > 1}
      <div class="flex items-center gap-1">
        {#each services as svc}
          <button
            onclick={() => switchService(svc)}
            class="px-2 py-1 text-xs rounded border transition-colors
              {selectedService === svc ? 'border-accent text-accent bg-accent/10' : 'border-border text-text-secondary hover:text-text-primary'}"
          >
            {svc}
          </button>
        {/each}
      </div>
      <div class="w-px h-4 bg-border"></div>
    {/if}
    <button
      onclick={toggleFollow}
      class="px-2 py-1 text-xs rounded border transition-colors
        {following ? 'border-success text-success' : 'border-border text-text-secondary hover:text-text-primary'}"
    >
      {following ? 'Following' : 'Paused'}
    </button>
    <button onclick={clear} class="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors">
      Clear
    </button>
    <button
      onclick={() => showTimestamps = !showTimestamps}
      class="px-2 py-1 text-xs rounded border transition-colors
        {showTimestamps ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
    >
      Timestamps
    </button>
    <button onclick={downloadLogs} class="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors">
      Download
    </button>
    <span class="ml-auto text-xs text-text-muted">{lines.length} lines</span>
  </div>

  <div
    bind:this={container}
    class="flex-1 overflow-y-auto bg-surface-0 border border-t-0 border-border rounded-b-lg font-mono text-xs p-3 space-y-px"
  >
    {#each lines as line}
      <div class="whitespace-pre-wrap break-all {line.stream === 'stderr' ? 'text-danger' : 'text-text-primary'}">
        {#if showTimestamps && line.ts}<span class="text-text-muted mr-2">{line.ts}</span>{/if}<span>{line.line}</span>
      </div>
    {/each}
  </div>
</div>
