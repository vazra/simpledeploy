<script>
  import { api } from '../lib/api.js'

  let { open = false, onclose = () => {}, onselect = () => {} } = $props()

  let loading = $state(false)
  let error = $state('')
  let recipes = $state([])
  let q = $state('')
  let category = $state('')
  let selected = $state(null)
  let detailLoading = $state(false)
  let detailReadme = $state('')
  let loaded = $state(false)
  let applying = $state(false)

  $effect(() => {
    if (open && !loaded && !loading) {
      load()
    }
  })

  async function load() {
    loading = true
    error = ''
    const { data, error: err } = await api.listCommunityRecipes()
    loading = false
    loaded = true
    if (err) { error = err; return }
    recipes = data?.recipes || []
  }

  let filtered = $derived(recipes.filter((r) => {
    if (category && r.category !== category) return false
    if (!q) return true
    const needle = q.toLowerCase()
    const hay = (r.name + ' ' + (r.description || '') + ' ' + (r.tags || []).join(' ')).toLowerCase()
    return hay.includes(needle)
  }))

  async function openDetail(r) {
    selected = r
    detailLoading = true
    detailReadme = ''
    const { data, error: err } = await api.fetchCommunityRecipeFile(r.id, 'readme')
    detailLoading = false
    detailReadme = err ? `Could not load README: ${err}` : (data || '')
  }

  async function useRecipe() {
    if (!selected || applying) return
    applying = true
    const { data, error: err } = await api.fetchCommunityRecipeFile(selected.id, 'compose')
    applying = false
    if (err) { error = err; return }
    const picked = { id: selected.id, name: selected.name, compose: data }
    onselect(picked)
    close()
  }

  function close() {
    selected = null
    detailReadme = ''
    error = ''
    onclose()
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4" role="dialog" aria-label="Browse community recipes">
    <div class="bg-bg-elevated rounded-xl shadow-xl max-w-4xl w-full max-h-[85vh] overflow-hidden flex flex-col border border-border/50">
      <header class="flex items-center justify-between px-5 py-3 border-b border-border/50">
        <h2 class="text-lg font-semibold text-text-primary">Community Recipes</h2>
        <button class="text-text-muted hover:text-text-primary" onclick={close} aria-label="Close">✕</button>
      </header>

      <div class="flex-1 overflow-auto p-5">
        {#if loading}
          <p class="text-text-muted">Loading recipes...</p>
        {:else if error && !selected}
          <p class="text-danger">Could not load recipes: {error}</p>
        {:else if !selected}
          <div class="flex gap-2 mb-4">
            <input
              type="search"
              placeholder="Search recipes"
              bind:value={q}
              class="flex-1 px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30"
            />
            <select bind:value={category} class="px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
              <option value="">All categories</option>
              <option value="web">Web</option>
              <option value="dev-tools">Dev Tools</option>
              <option value="databases">Databases</option>
              <option value="storage">Storage</option>
              <option value="productivity">Productivity</option>
              <option value="observability">Observability</option>
              <option value="auth">Auth</option>
              <option value="mail">Mail</option>
              <option value="ci">CI/CD</option>
            </select>
          </div>
          {#if filtered.length === 0}
            <p class="text-text-muted text-sm">No recipes match.</p>
          {:else}
            <ul class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 list-none p-0">
              {#each filtered as r (r.id)}
                <li>
                  <button
                    type="button"
                    onclick={() => openDetail(r)}
                    class="w-full text-left p-3 rounded-lg border border-border/50 bg-surface-3/40 hover:border-accent/50 transition flex flex-col gap-1"
                  >
                    <span class="flex items-center gap-2">
                      <span class="text-xl">{r.icon || '📦'}</span>
                      <span class="font-medium text-text-primary text-sm">{r.name}</span>
                    </span>
                    <span class="text-xs text-text-muted line-clamp-2">{r.description}</span>
                    {#if r.author}<span class="text-[10px] text-text-muted">by {r.author}</span>{/if}
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {:else}
          <div class="flex flex-col gap-3">
            <button class="self-start text-sm text-text-muted hover:text-text-primary" onclick={() => { selected = null; detailReadme = '' }}>← Back to list</button>
            <h3 class="text-xl font-semibold text-text-primary flex items-center gap-2">
              <span>{selected.icon || '📦'}</span>
              <span>{selected.name}</span>
            </h3>
            <p class="text-sm text-text-muted">{selected.description}</p>
            {#if detailLoading}
              <p class="text-text-muted text-sm">Loading README...</p>
            {:else}
              <pre class="bg-surface-3/40 p-3 rounded-lg border border-border/50 text-xs text-text-primary whitespace-pre-wrap font-mono max-h-80 overflow-auto">{detailReadme}</pre>
            {/if}
            {#if error}
              <p class="text-xs text-danger">{error}</p>
            {/if}
            <div class="flex justify-end">
              <button
                type="button"
                onclick={useRecipe}
                disabled={applying}
                class="px-4 py-2 bg-accent hover:bg-accent/90 text-white rounded-lg text-sm font-medium disabled:opacity-50"
              >
                {applying ? 'Loading...' : 'Use Recipe'}
              </button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}
