<script>
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'
  import ActivityRow from './ActivityRow.svelte'

  // Polling cadence: 10s when visible, paused when hidden.
  const POLL_MS = 10_000
  // Auto-close after this idle window (ms with no new entries) once auto-opened.
  const AUTO_CLOSE_MS = 8_000

  let entries = $state([])
  let open = $state(false)
  let manuallyToggled = $state(false)
  let lastSeenId = $state(0)
  let unseenCount = $state(0)
  let pollTimer
  let closeTimer

  async function refresh() {
    if (document.visibilityState !== 'visible') return
    const { data } = await api.listRecentActivity(20)
    const next = data?.entries || []
    if (next.length === 0) return
    const top = next[0].id
    if (lastSeenId !== 0 && top > lastSeenId) {
      // New activity since last fetch.
      const newCount = next.findIndex((e) => e.id <= lastSeenId)
      const incoming = newCount === -1 ? next.length : newCount
      unseenCount += incoming
      if (!manuallyToggled || !open) {
        autoOpen()
      }
    }
    entries = next
    lastSeenId = top
  }

  function autoOpen() {
    open = true
    scheduleAutoClose()
  }

  function scheduleAutoClose() {
    clearTimeout(closeTimer)
    closeTimer = setTimeout(() => {
      // Only auto-close if user didn't manually pin it open.
      if (!manuallyToggled) {
        open = false
      }
    }, AUTO_CLOSE_MS)
  }

  function toggle() {
    open = !open
    manuallyToggled = open
    if (open) {
      unseenCount = 0
      clearTimeout(closeTimer)
    }
  }

  function close() {
    open = false
    manuallyToggled = false
    unseenCount = 0
    clearTimeout(closeTimer)
  }

  function onVisibility() {
    if (document.visibilityState === 'visible') refresh()
  }

  onMount(() => {
    refresh()
    pollTimer = setInterval(refresh, POLL_MS)
    document.addEventListener('visibilitychange', onVisibility)
  })

  onDestroy(() => {
    clearInterval(pollTimer)
    clearTimeout(closeTimer)
    document.removeEventListener('visibilitychange', onVisibility)
  })
</script>

<!-- Floating toggle button (always visible, top-right) -->
<button
  data-testid="activity-sidebar-toggle"
  onclick={toggle}
  aria-label="Toggle activity sidebar"
  title="Recent activity"
  class="fixed top-3 right-3 z-50 inline-flex items-center justify-center w-9 h-9 rounded-full bg-surface-2 border border-border/50 text-text-secondary hover:text-text-primary hover:border-border shadow-sm transition-colors"
>
  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
    <path stroke-linecap="round" stroke-linejoin="round" d="M15 17h5l-1.4-1.4A2 2 0 0118 14.2V11a6 6 0 10-12 0v3.2c0 .5-.2 1-.6 1.4L4 17h5m6 0a3 3 0 11-6 0" />
  </svg>
  {#if unseenCount > 0}
    <span class="absolute -top-1 -right-1 inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-accent text-white text-[10px] font-semibold">
      {unseenCount > 99 ? '99+' : unseenCount}
    </span>
  {/if}
</button>

<!-- Slide-in sidebar -->
<aside
  data-testid="activity-sidebar"
  class="fixed top-0 right-0 h-full w-80 max-w-[90vw] bg-surface-1 border-l border-border/40 shadow-xl z-40 transform transition-transform duration-200 ease-out flex flex-col"
  class:translate-x-0={open}
  class:translate-x-full={!open}
  aria-hidden={!open}
>
  <header class="flex items-center justify-between px-4 py-3 border-b border-border/40">
    <h3 class="text-sm font-semibold text-text-primary">Recent Activity</h3>
    <div class="flex items-center gap-1">
      <a href="#/system?tab=audit" class="text-xs text-accent hover:underline mr-1">View all</a>
      <button
        onclick={close}
        aria-label="Close activity sidebar"
        class="text-text-muted hover:text-text-primary p-1 rounded"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 6l12 12M6 18L18 6" />
        </svg>
      </button>
    </div>
  </header>

  <div class="flex-1 overflow-y-auto px-3 py-2">
    {#if entries.length === 0}
      <p class="text-xs text-text-secondary italic">No activity yet.</p>
    {:else}
      <div class="flex flex-col divide-y divide-border/20">
        {#each entries as e (e.id)}
          <ActivityRow entry={e} compact showAppColumn />
        {/each}
      </div>
    {/if}
  </div>
</aside>
