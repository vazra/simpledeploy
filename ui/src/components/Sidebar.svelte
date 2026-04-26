<script>
  import { onMount } from 'svelte'
  import { sidebarExpanded } from '../lib/stores/sidebar.js'
  import ThemeToggle from './ThemeToggle.svelte'
  import HelpMenu from './HelpMenu.svelte'
  import { api } from '../lib/api.js'

  let { forceExpanded = false } = $props()
  let currentPath = $state(window.location.hash.slice(1) || '/')
  let profile = $state(null)
  let helpOpen = $state(false)

  function initials(name) {
    if (!name) return '?'
    return name.split(' ').map(w => w[0]).join('').toUpperCase().slice(0, 2)
  }

  const nav = [
    { path: '/', label: 'Dashboard', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" /></svg>' },
    { path: '/alerts', label: 'Alerts', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" /></svg>' },
    { path: '/backups', label: 'Backups', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5m8.25 3v6.75m0 0l-3-3m3 3l3-3M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" /></svg>' },
    { path: '/users', label: 'Users', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" /></svg>' },
    { path: '/registries', label: 'Registries', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" /></svg>' },
    { path: '/docker', label: 'Docker', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg>' },
    { path: '/system', label: 'System', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-13.5 0v-1.5m13.5 1.5v-1.5m0 0a3 3 0 01-3-3m3 3a3 3 0 000-6H5.25a3 3 0 000 6m13.5 0v1.5a3 3 0 01-3 3H8.25a3 3 0 01-3-3v-1.5m13.5-1.5H5.25m8.25 3h.008v.008h-.008V17.25zm-3 0h.008v.008h-.008V17.25z" /></svg>' },
    { path: '/git-sync', label: 'Git Sync', requiresSuperAdmin: true, icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" /></svg>' },
  ]

  function updatePath() {
    currentPath = window.location.hash.slice(1) || '/'
  }

  onMount(() => {
    window.addEventListener('hashchange', updatePath)
    api.getProfile().then(res => { if (res.data) profile = res.data })
    return () => window.removeEventListener('hashchange', updatePath)
  })

  function isActive(path) {
    if (path === '/') return currentPath === '/'
    return currentPath.startsWith(path)
  }

  function toggle() {
    sidebarExpanded.update((v) => !v)
  }
</script>

<aside class="flex flex-col h-screen sticky top-0 shrink-0 bg-surface-1 border-r border-border/30 transition-all duration-200 {forceExpanded || $sidebarExpanded ? 'w-56' : 'w-16'}">
  <div class="flex items-center h-16 px-4 border-b border-border/30">
    <div class="flex items-center gap-2 overflow-hidden">
      <svg class="w-7 h-7 shrink-0 text-accent" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
      </svg>
      {#if forceExpanded || $sidebarExpanded}
        <span class="text-sm font-semibold text-text-primary whitespace-nowrap">SimpleDeploy</span>
      {/if}
    </div>
  </div>

  <nav class="flex-1 flex flex-col gap-1 py-3 px-3">
    {#each nav.filter(i => !i.requiresSuperAdmin || profile?.role === 'super_admin') as item}
      <a
        href="#{item.path}"
        class="flex items-center gap-2.5 px-3 py-2.5 rounded-lg relative text-sm transition-colors
          {isActive(item.path) ? 'bg-surface-3/50 text-text-primary font-medium' : 'text-text-secondary hover:text-text-primary hover:bg-surface-3/30'}"
        title={forceExpanded || $sidebarExpanded ? '' : item.label}
      >
        {#if isActive(item.path)}
          <span class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-4 bg-accent rounded-full"></span>
        {/if}
        <span class="shrink-0">{@html item.icon}</span>
        {#if forceExpanded || $sidebarExpanded}
          <span class="whitespace-nowrap">{item.label}</span>
        {/if}
      </a>
    {/each}
  </nav>

  <div class="flex flex-col gap-1 p-3 border-t border-border/30">
    <a
      href="#/profile"
      class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors
        {isActive('/profile') ? 'bg-surface-3/50 text-text-primary font-medium' : 'text-text-secondary hover:text-text-primary hover:bg-surface-3/30'}"
      title={forceExpanded || $sidebarExpanded ? '' : (profile?.display_name || profile?.username || 'Profile')}
    >
      <span class="w-8 h-8 shrink-0 rounded-full bg-accent/15 text-accent flex items-center justify-center text-xs font-semibold">
        {initials(profile?.display_name || profile?.username)}
      </span>
      {#if forceExpanded || $sidebarExpanded}
        <span class="flex flex-col min-w-0">
          <span class="text-sm font-medium text-text-primary truncate">{profile?.display_name || profile?.username || ''}</span>
          {#if profile?.display_name}
            <span class="text-[11px] text-text-muted truncate">{profile?.username}</span>
          {/if}
        </span>
      {/if}
    </a>
    <button
      onclick={() => (helpOpen = true)}
      class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm text-text-secondary hover:text-text-primary hover:bg-surface-3/30 transition-colors"
      title={forceExpanded || $sidebarExpanded ? '' : 'Help & feedback'}
      aria-label="Help and feedback"
    >
      <span class="shrink-0">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z" />
        </svg>
      </span>
      {#if forceExpanded || $sidebarExpanded}
        <span class="whitespace-nowrap">Help & feedback</span>
      {/if}
    </button>
    <div class="flex items-center justify-center">
      <ThemeToggle />
    </div>
    <button
      onclick={toggle}
      class="flex items-center justify-center w-full py-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3/50 transition-colors"
      title={$sidebarExpanded ? 'Collapse sidebar' : 'Expand sidebar'}
      aria-label="Toggle sidebar"
    >
      <svg class="w-4 h-4 transition-transform {$sidebarExpanded ? '' : 'rotate-180'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
      </svg>
    </button>
  </div>
</aside>

{#if helpOpen}
  <HelpMenu onClose={() => (helpOpen = false)} />
{/if}
