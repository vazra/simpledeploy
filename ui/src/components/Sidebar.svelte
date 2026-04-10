<script>
  import { onMount } from 'svelte'
  import { sidebarExpanded } from '../lib/stores/sidebar.js'
  import ThemeToggle from './ThemeToggle.svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'

  let currentPath = $state(window.location.hash.slice(1) || '/')

  const nav = [
    { path: '/', label: 'Dashboard', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" /></svg>' },
    { path: '/alerts', label: 'Alerts', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" /></svg>' },
    { path: '/backups', label: 'Backups', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5m8.25 3v6.75m0 0l-3-3m3 3l3-3M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" /></svg>' },
    { path: '/users', label: 'Users', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" /></svg>' },
    { path: '/registries', label: 'Registries', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" /></svg>' },
  ]

  function updatePath() {
    currentPath = window.location.hash.slice(1) || '/'
  }

  onMount(() => {
    window.addEventListener('hashchange', updatePath)
    return () => window.removeEventListener('hashchange', updatePath)
  })

  function isActive(path) {
    if (path === '/') return currentPath === '/'
    return currentPath.startsWith(path)
  }

  async function logout() {
    await api.logout()
    push('/login')
  }

  function toggle() {
    sidebarExpanded.update((v) => !v)
  }
</script>

<aside class="flex flex-col h-screen bg-surface-1 border-r border-border transition-all duration-200 {$sidebarExpanded ? 'w-52' : 'w-14'}">
  <div class="flex items-center h-14 px-3 border-b border-border">
    <div class="flex items-center gap-2 overflow-hidden">
      <svg class="w-7 h-7 shrink-0 text-accent" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
      </svg>
      {#if $sidebarExpanded}
        <span class="text-sm font-semibold text-accent whitespace-nowrap">SimpleDeploy</span>
      {/if}
    </div>
  </div>

  <nav class="flex-1 flex flex-col gap-0.5 py-2 px-2">
    {#each nav as item}
      <a
        href="#{item.path}"
        class="flex items-center gap-2.5 px-2 py-2 rounded-md text-sm transition-colors
          {isActive(item.path) ? 'bg-surface-3 text-text-primary' : 'text-text-secondary hover:text-text-primary hover:bg-surface-3/50'}"
        title={$sidebarExpanded ? '' : item.label}
      >
        <span class="shrink-0">{@html item.icon}</span>
        {#if $sidebarExpanded}
          <span class="whitespace-nowrap">{item.label}</span>
        {/if}
      </a>
    {/each}
  </nav>

  <div class="flex flex-col gap-1 p-2 border-t border-border">
    <div class="flex items-center {$sidebarExpanded ? 'justify-between' : 'justify-center'}">
      <ThemeToggle />
      {#if $sidebarExpanded}
        <button
          onclick={logout}
          class="text-xs text-text-secondary hover:text-danger transition-colors"
        >
          Logout
        </button>
      {/if}
    </div>
    <button
      onclick={toggle}
      class="flex items-center justify-center w-full py-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-surface-3/50 transition-colors"
      title={$sidebarExpanded ? 'Collapse sidebar' : 'Expand sidebar'}
      aria-label="Toggle sidebar"
    >
      <svg class="w-4 h-4 transition-transform {$sidebarExpanded ? '' : 'rotate-180'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
      </svg>
    </button>
  </div>
</aside>
