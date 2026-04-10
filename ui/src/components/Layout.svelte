<script>
  import Sidebar from './Sidebar.svelte'
  import { connection } from '../lib/stores/connection.svelte.js'

  let { children } = $props()
</script>

<div class="flex min-h-screen bg-surface-0">
  <Sidebar />
  <main class="flex-1 overflow-y-auto">
    {#if !connection.connected}
      <div class="bg-red-900/40 border-b border-red-800 px-6 py-2 text-sm text-red-300 flex items-center gap-2 light:bg-red-50 light:border-red-200 light:text-red-700">
        <span class="w-2 h-2 rounded-full bg-red-500 animate-pulse"></span>
        Backend unavailable
        <button onclick={() => connection.check()} class="ml-auto text-xs underline hover:no-underline">Retry</button>
      </div>
    {/if}
    <div class="p-6">
      {@render children()}
    </div>
  </main>
</div>
