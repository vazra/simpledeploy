<script>
  import { onMount, onDestroy } from 'svelte'

  let { slug, service = '' } = $props()

  let lines = $state([])
  let ws = $state(null)
  let following = $state(true)
  let container

  onMount(() => { connect() })
  onDestroy(() => { if (ws) ws.close() })

  function connect() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    let url = `${proto}//${window.location.host}/api/apps/${slug}/logs?follow=true&tail=200`
    if (service) url += `&service=${service}`

    ws = new WebSocket(url)
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      lines = [...lines.slice(-999), msg]
      if (following && container) {
        requestAnimationFrame(() => {
          container.scrollTop = container.scrollHeight
        })
      }
    }
    ws.onclose = () => { ws = null }
  }

  function toggleFollow() {
    following = !following
    if (following && container) {
      container.scrollTop = container.scrollHeight
    }
  }

  function clear() { lines = [] }
</script>

<div class="log-viewer">
  <div class="log-toolbar">
    <button onclick={toggleFollow} class:active={following}>
      {following ? 'Following' : 'Paused'}
    </button>
    <button onclick={clear}>Clear</button>
    <span class="line-count">{lines.length} lines</span>
  </div>
  <div class="log-container" bind:this={container}>
    {#each lines as line}
      <div class="log-line" class:stderr={line.stream === 'stderr'}>
        {#if line.ts}<span class="ts">{line.ts}</span>{/if}
        <span class="text">{line.line}</span>
      </div>
    {/each}
  </div>
</div>

<style>
  .log-viewer { display: flex; flex-direction: column; height: 500px; }
  .log-toolbar {
    display: flex; gap: 0.5rem; align-items: center;
    padding: 0.5rem; background: #161b22; border: 1px solid #21262d;
    border-radius: 4px 4px 0 0;
  }
  .log-toolbar button {
    padding: 0.3rem 0.6rem; background: #21262d; border: 1px solid #30363d;
    border-radius: 4px; color: #8b949e; cursor: pointer; font-size: 0.75rem;
  }
  .log-toolbar button.active { color: #3fb950; border-color: #3fb950; }
  .line-count { margin-left: auto; color: #484f58; font-size: 0.75rem; }
  .log-container {
    flex: 1; overflow-y: auto; background: #0d1117;
    border: 1px solid #21262d; border-top: none;
    border-radius: 0 0 4px 4px; font-family: 'SF Mono', monospace; font-size: 0.8rem;
    padding: 0.5rem;
  }
  .log-line { padding: 1px 0; white-space: pre-wrap; word-break: break-all; }
  .log-line.stderr { color: #f85149; }
  .ts { color: #484f58; margin-right: 0.5rem; }
  .text { color: #c9d1d9; }
</style>
