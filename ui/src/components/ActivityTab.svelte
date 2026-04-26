<script>
  import { api } from '../lib/api.js'
  import ActivityRow from './ActivityRow.svelte'
  import Skeleton from './Skeleton.svelte'

  let { slug } = $props()

  let entries = $state([])
  let nextBefore = $state(0)
  let categories = $state([])
  let loading = $state(false)

  const allCats = ['compose', 'env', 'endpoint', 'backup', 'alert', 'webhook', 'registry', 'access', 'deploy', 'lifecycle']

  async function load(reset = false) {
    loading = true
    try {
      if (reset) { entries = []; nextBefore = 0 }
      const { data } = await api.listAppActivity(slug, { categories, before: nextBefore })
      const next = data || {}
      entries = reset ? (next.entries || []) : [...entries, ...(next.entries || [])]
      nextBefore = next.next_before || 0
    } finally {
      loading = false
    }
  }

  function toggleCat(c) {
    categories = categories.includes(c) ? categories.filter(x => x !== c) : [...categories, c]
    load(true)
  }

  $effect(() => {
    if (slug) load(true)
  })
</script>

<div>
  <div class="flex flex-wrap gap-2 mb-4">
    {#each allCats as c}
      <button
        data-testid="activity-filter-{c}"
        class="chip px-3 py-1 rounded-full text-xs font-medium border transition-colors {categories.includes(c) ? 'bg-accent text-white border-accent' : 'bg-surface-2 border-border/50 text-text-secondary hover:text-text-primary hover:border-border'}"
        onclick={() => toggleCat(c)}
      >{c}</button>
    {/each}
  </div>

  {#if loading && entries.length === 0}
    <Skeleton type="card" count={3} />
  {:else if entries.length === 0}
    <p class="text-sm text-text-muted italic">No activity yet. Changes will appear here.</p>
  {:else}
    <div class="space-y-2">
      {#each entries as e (e.id)}
        <ActivityRow entry={e} expandable />
      {/each}
    </div>
    {#if nextBefore > 0}
      <button
        class="mt-4 px-4 py-2 text-sm border border-border/50 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors disabled:opacity-50"
        onclick={() => load(false)}
        disabled={loading}
      >
        {loading ? 'Loading…' : 'Load more'}
      </button>
    {/if}
  {/if}
</div>
