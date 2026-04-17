<script>
  import { onMount } from 'svelte'
  import { statusBar } from '../lib/stores/statusbar.svelte.js'
  import { connection } from '../lib/stores/connection.svelte.js'
  import { formatBytes } from '../lib/format.js'

  const unsubReconnect = connection.onReconnect(() => statusBar.load())
  onMount(() => {
    if (!statusBar.loaded) statusBar.load()
    return unsubReconnect
  })
</script>

{#if statusBar.loaded}
<a href="#/docker" class="shrink-0 flex items-center gap-x-5 px-4 py-1.5 bg-surface-1 border-t border-border/30 text-[11px] text-text-muted hover:text-text-secondary transition-colors cursor-pointer">
  {#if statusBar.sysInfo?.simpledeploy}
    <span class="flex items-center gap-1.5">
      <svg class="w-3 h-3 text-accent" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
      SimpleDeploy {statusBar.sysInfo.simpledeploy.version || 'dev'}
    </span>
    <span title="Process memory usage">Mem: {formatBytes(statusBar.sysInfo.simpledeploy.process?.mem_alloc || 0)}</span>
  {/if}
  {#if statusBar.sysInfo?.database}
    <span title="Database size on disk">DB: {formatBytes(statusBar.sysInfo.database.size_bytes || 0)}</span>
  {/if}
  {#if statusBar.dockerInfo}
    <span class="flex items-center gap-1">
      <span class="w-1.5 h-1.5 rounded-full bg-success"></span>
      Docker Engine {statusBar.dockerInfo.server_version}
    </span>
    {#if statusBar.dockerInfo.compose_version}
      <span class="flex items-center gap-1">
        <span class="w-1.5 h-1.5 rounded-full bg-success"></span>
        Compose {statusBar.dockerInfo.compose_version}
      </span>
    {/if}
  {:else}
    <span class="flex items-center gap-1">
      <span class="w-1.5 h-1.5 rounded-full bg-danger"></span>
      Docker unavailable
    </span>
  {/if}
</a>
{/if}
