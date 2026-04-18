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

  const deploymentTitles = {
    native: 'Running as a native binary',
    docker: 'Running inside a Docker container (host networking)',
    'docker-desktop': 'Running inside a Docker Desktop container',
    'docker-dev': 'Running inside a contributor dev container',
  }
</script>

{#if statusBar.loaded}
<a href="#/docker" class="shrink-0 flex items-center gap-x-5 px-4 py-1.5 bg-surface-1 border-t border-border/30 text-[11px] text-text-muted hover:text-text-secondary transition-colors cursor-pointer">
  {#if statusBar.sysInfo?.simpledeploy}
    <span class="flex items-center gap-1.5">
      <svg class="w-3 h-3 text-accent" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
      SimpleDeploy {statusBar.sysInfo.simpledeploy.version || 'dev'}
    </span>
    {#if statusBar.sysInfo.simpledeploy.deployment_label}
      <span
        class="flex items-center gap-1.5"
        title={deploymentTitles[statusBar.sysInfo.simpledeploy.deployment_mode] || ''}
      >
        {#if statusBar.sysInfo.simpledeploy.deployment_mode === 'native'}
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></svg>
        {:else}
          <svg class="w-3.5 h-3" viewBox="0 0 24 14" fill="currentColor"><path d="M23.1 6.3c-.06-.04-.6-.43-1.74-.43-.3 0-.62.03-.93.08-.23-1.6-1.55-2.38-1.6-2.42l-.33-.19-.21.31c-.27.42-.47.89-.58 1.37-.22.95-.08 1.84.39 2.6-.57.32-1.48.4-1.67.4H.63a.63.63 0 0 0-.63.62 9.46 9.46 0 0 0 .58 3.4c.46 1.2 1.14 2.08 2.03 2.62C3.5 14.7 5.1 15 6.88 15c.8 0 1.6-.07 2.39-.22 1.1-.2 2.16-.58 3.14-1.12a8.64 8.64 0 0 0 2.14-1.74c1.05-1.18 1.68-2.5 2.14-3.66h.2c1.22 0 1.97-.49 2.39-.9.28-.27.5-.58.65-.95l.08-.24-.22-.12z"/></svg>
        {/if}
        {statusBar.sysInfo.simpledeploy.deployment_label}
      </span>
    {/if}
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
