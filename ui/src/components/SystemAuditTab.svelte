<script>
  import { api } from '../lib/api.js'
  import { toasts } from '../lib/stores/toast.js'
  import ActivityRow from './ActivityRow.svelte'
  import Skeleton from './Skeleton.svelte'

  let { isSuperAdmin = false } = $props()

  const allCats = ['auth', 'user', 'app', 'compose', 'deploy', 'backup', 'system', 'endpoint', 'alert', 'webhook', 'registry', 'access']

  let entries = $state([])
  let nextBefore = $state(0)
  let categories = $state([])
  let appFilter = $state('')
  let apps = $state([])
  let retentionDays = $state(0)
  let loading = $state(false)
  let purging = $state(false)
  let savingConfig = $state(false)
  let showPurgeConfirm = $state(false)

  $effect(() => {
    loadApps()
    loadAuditConfig()
    load(true)
  })

  async function loadApps() {
    const res = await api.listApps()
    if (res.data) apps = res.data
  }

  async function loadAuditConfig() {
    const res = await api.getAuditConfig()
    if (res.data) retentionDays = res.data.retention_days ?? 0
  }

  async function load(reset = false) {
    loading = true
    try {
      if (reset) { entries = []; nextBefore = 0 }
      const res = await api.listActivity({
        categories,
        app: appFilter || undefined,
        before: reset ? 0 : nextBefore,
      })
      const fetched = res.data?.entries || res.entries || []
      entries = reset ? fetched : [...entries, ...fetched]
      nextBefore = res.data?.next_before ?? res.next_before ?? 0
    } finally {
      loading = false
    }
  }

  function toggleCat(c) {
    categories = categories.includes(c) ? categories.filter(x => x !== c) : [...categories, c]
    load(true)
  }

  async function saveRetention() {
    savingConfig = true
    const res = await api.putAuditConfig({ retention_days: retentionDays })
    if (res.error) toasts.error(res.error)
    else toasts.success('Retention saved')
    savingConfig = false
  }

  async function purgeAll() {
    showPurgeConfirm = false
    purging = true
    const res = await api.purgeActivity()
    if (res.error) toasts.error(res.error)
    else {
      toasts.success('Activity purged')
      load(true)
    }
    purging = false
  }
</script>

<div class="space-y-4">
  <!-- Filter row -->
  <div class="flex flex-wrap gap-2 items-center">
    {#each allCats as c}
      <button
        class="chip px-3 py-1 rounded-full text-xs font-medium border transition-colors {categories.includes(c) ? 'bg-accent text-white border-accent' : 'bg-surface-2 border-border/50 text-text-secondary hover:text-text-primary hover:border-border'}"
        onclick={() => toggleCat(c)}
      >{c}</button>
    {/each}

    <select
      bind:value={appFilter}
      onchange={() => load(true)}
      class="ml-auto px-2 py-1 text-xs bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
    >
      <option value="">All apps</option>
      <option value="system">system</option>
      {#each apps as app}
        <option value={app.Slug ?? app.slug}>{app.Name ?? app.name ?? app.Slug ?? app.slug}</option>
      {/each}
    </select>
  </div>

  <!-- Entry list -->
  {#if loading && entries.length === 0}
    <Skeleton type="card" count={4} />
  {:else if entries.length === 0}
    <div class="bg-surface-2 rounded-xl p-8 shadow-sm border border-border/50 text-center">
      <p class="text-sm text-text-secondary">No activity yet.</p>
    </div>
  {:else}
    <div class="space-y-2">
      {#each entries as e (e.id)}
        <ActivityRow entry={e} expandable showAppColumn />
      {/each}
    </div>

    {#if nextBefore > 0}
      <button
        class="mt-2 px-4 py-2 text-sm border border-border/50 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors disabled:opacity-50"
        onclick={() => load(false)}
        disabled={loading}
      >{loading ? 'Loading...' : 'Load more'}</button>
    {/if}
  {/if}

  <!-- Super-admin controls -->
  {#if isSuperAdmin}
    <div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50 space-y-4">
      <!-- Retention config -->
      <div>
        <h3 class="text-sm font-semibold text-text-primary mb-1">Retention</h3>
        <p class="text-xs text-text-secondary mb-3">Keep activity for N days (0 = forever).</p>
        <div class="flex flex-wrap items-center gap-3">
          <input
            type="number"
            min="0"
            bind:value={retentionDays}
            class="w-24 px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent"
          />
          <span class="text-xs text-text-secondary">days</span>
          <button
            class="px-3 py-1.5 text-xs font-medium rounded-lg border border-border/50 bg-surface-1 text-text-primary hover:bg-surface-3 transition-colors disabled:opacity-50"
            onclick={saveRetention}
            disabled={savingConfig}
          >{savingConfig ? 'Saving...' : 'Save'}</button>
        </div>
      </div>

      <!-- Purge -->
      <div class="border-t border-border/30 pt-4">
        <h3 class="text-sm font-semibold text-text-primary mb-1">Purge</h3>
        <p class="text-xs text-text-secondary mb-3">Permanently delete all activity records.</p>
        {#if showPurgeConfirm}
          <div class="flex items-center gap-2">
            <span class="text-xs text-text-secondary">Are you sure?</span>
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-lg bg-red-500 text-white hover:bg-red-600 transition-colors disabled:opacity-50"
              onclick={purgeAll}
              disabled={purging}
            >{purging ? 'Purging...' : 'Yes, purge all'}</button>
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-lg border border-border/50 bg-surface-1 text-text-secondary hover:text-text-primary transition-colors"
              onclick={() => showPurgeConfirm = false}
            >Cancel</button>
          </div>
        {:else}
          <button
            class="px-3 py-1.5 text-xs font-medium rounded-lg border border-red-500/50 text-red-400 hover:bg-red-500/10 transition-colors"
            onclick={() => showPurgeConfirm = true}
          >Purge all activity</button>
        {/if}
      </div>
    </div>
  {/if}
</div>
