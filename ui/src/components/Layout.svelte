<script>
  import Sidebar from './Sidebar.svelte'
  import StatusBar from './StatusBar.svelte'
  import { connection } from '../lib/stores/connection.svelte.js'

  let { children } = $props()
  let mobileMenuOpen = $state(false)

  function closeMobileMenu() {
    mobileMenuOpen = false
  }
</script>

<svelte:window onhashchange={closeMobileMenu} />

<div class="flex min-h-screen bg-surface-0">
  <!-- Desktop sidebar -->
  <div class="hidden md:block">
    <Sidebar />
  </div>

  <!-- Mobile overlay sidebar -->
  {#if mobileMenuOpen}
    <div class="fixed inset-0 z-50 md:hidden">
      <button class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={closeMobileMenu} aria-label="Close menu"></button>
      <div class="relative w-56 h-full animate-slide-panel" style="animation-name: slideFromLeft">
        <Sidebar forceExpanded={true} />
      </div>
    </div>
  {/if}

  <main class="flex-1 flex flex-col overflow-hidden min-w-0">
    <!-- Mobile header -->
    <div class="flex md:hidden items-center gap-3 px-4 py-3 border-b border-border/30 bg-surface-1 sticky top-0 z-40">
      <button onclick={() => mobileMenuOpen = true} class="text-text-secondary hover:text-text-primary" aria-label="Open menu">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
        </svg>
      </button>
      <div class="flex items-center gap-2">
        <svg class="w-5 h-5 text-accent" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
        </svg>
        <span class="text-sm font-semibold text-text-primary">SimpleDeploy</span>
      </div>
    </div>

    {#if !connection.connected}
      <div class="bg-red-500/10 border-b border-red-500/20 px-6 py-2.5 text-sm text-red-400 flex items-center gap-2 light:bg-red-50 light:border-red-100 light:text-red-600">
        <span class="w-2 h-2 rounded-full bg-red-500 animate-pulse"></span>
        Backend unavailable
        <button onclick={() => connection.check()} class="ml-auto text-xs underline hover:no-underline">Retry</button>
      </div>
    {/if}
    <div class="flex-1 overflow-y-auto p-4 md:p-8">
      {@render children()}
    </div>
    <StatusBar />
  </main>
</div>

<style>
  @keyframes slideFromLeft {
    from { transform: translateX(-100%); }
    to { transform: translateX(0); }
  }
</style>
