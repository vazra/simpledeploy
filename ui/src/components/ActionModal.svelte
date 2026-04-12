<script>
  import { onDestroy } from 'svelte'
  import { api } from '../lib/api.js'
  import Badge from './Badge.svelte'

  let { slug, action, show, onclose } = $props()

  let liveLines = $state([])
  let liveStatus = $state(null)
  let ws = null
  let terminalEl = $state(null)

  $effect(() => {
    if (show) {
      connect()
    } else {
      disconnect()
    }
  })

  onDestroy(disconnect)

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

  function handleClose() {
    disconnect()
    onclose()
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && liveStatus) handleClose()
  }

</script>

<svelte:window onkeydown={onKeydown} />

{#if show}
  <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
    <button
      class="absolute inset-0 bg-black/50 backdrop-blur-sm"
      onclick={() => { if (liveStatus) handleClose() }}
      aria-label="Close"
    ></button>

    <div class="relative bg-surface-2 border border-border/50 rounded-2xl shadow-2xl animate-scale-in w-full max-w-2xl mx-4">
      <!-- Header -->
      <div class="flex items-center gap-2.5 px-5 py-3.5 border-b border-border/50">
        {#if !liveStatus}
          <span class="w-2 h-2 rounded-full bg-blue-400 animate-pulse shrink-0"></span>
          <span class="text-sm font-medium text-text-primary">{action}...</span>
        {:else}
          {@const failed = liveStatus.action?.includes('failed')}
          <span class="w-2 h-2 rounded-full {failed ? 'bg-danger' : 'bg-success'} shrink-0"></span>
          <span class="text-sm font-medium text-text-primary">{failed ? 'Failed' : 'Complete'}</span>
          <Badge variant={failed ? 'danger' : 'success'}>{liveStatus.action}</Badge>
        {/if}
      </div>

      <!-- Terminal -->
      <div
        bind:this={terminalEl}
        class="bg-[#0c0c0c] light:bg-[#1a1a2e] font-mono text-[13px] leading-5 p-4 overflow-y-auto max-h-[400px] selection:bg-accent/30 {liveStatus ? '' : 'rounded-b-2xl'}"
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

      <!-- Footer (only when done) -->
      {#if liveStatus}
        <div class="flex justify-end px-5 py-3.5 border-t border-border/50">
          <button
            onclick={handleClose}
            class="px-4 py-2 text-sm bg-surface-3 border border-border/50 rounded-lg text-text-primary hover:bg-surface-2 transition-colors"
          >
            Close
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
