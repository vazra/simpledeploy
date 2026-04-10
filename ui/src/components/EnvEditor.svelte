<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import Button from './Button.svelte'

  let { slug } = $props()

  let vars = $state([])
  let loading = $state(true)
  let saving = $state(false)
  let showValues = $state(false)

  onMount(async () => {
    const res = await api.getEnv(slug)
    vars = res.data || []
    loading = false
  })

  function addVar() {
    vars = [...vars, { key: '', value: '' }]
  }

  function removeVar(i) {
    vars = vars.filter((_, idx) => idx !== i)
  }

  function updateKey(i, val) {
    vars = vars.map((v, idx) => idx === i ? { ...v, key: val } : v)
  }

  function updateValue(i, val) {
    vars = vars.map((v, idx) => idx === i ? { ...v, value: val } : v)
  }

  async function save() {
    saving = true
    await api.putEnv(slug, vars)
    saving = false
  }
</script>

<div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
  <div class="flex items-center justify-between mb-3">
    <div>
      <h3 class="text-sm font-semibold text-text-primary">Environment Variables</h3>
      <p class="text-xs text-text-secondary mt-0.5">Stored in <code class="font-mono">.env</code> alongside your compose file. Docker Compose loads these automatically.</p>
    </div>
    <div class="flex gap-2">
      <button
        onclick={() => showValues = !showValues}
        class="px-2 py-1 text-xs rounded-lg border border-border/50 text-text-secondary hover:text-text-primary transition-colors"
      >
        {showValues ? 'Hide values' : 'Show values'}
      </button>
    </div>
  </div>

  {#if loading}
    <p class="text-xs text-text-muted">Loading...</p>
  {:else}
    {#if vars.length > 0}
      <div class="overflow-x-auto mb-3">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4 w-1/3">Key</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Value</th>
              <th class="py-3 px-4 w-8"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/30">
            {#each vars as v, i}
              <tr class="hover:bg-surface-hover">
                <td class="py-1.5 px-2">
                  <input
                    type="text"
                    value={v.key}
                    oninput={(e) => updateKey(i, e.currentTarget.value)}
                    placeholder="KEY"
                    class="w-full bg-transparent font-mono text-xs text-text-primary outline-none focus:bg-input-bg px-1 py-0.5 rounded"
                  />
                </td>
                <td class="py-1.5 px-2">
                  <input
                    type={showValues ? 'text' : 'password'}
                    value={v.value}
                    oninput={(e) => updateValue(i, e.currentTarget.value)}
                    placeholder="value"
                    class="w-full bg-transparent font-mono text-xs text-text-primary outline-none focus:bg-input-bg px-1 py-0.5 rounded"
                  />
                </td>
                <td class="py-1.5 px-2">
                  <button
                    onclick={() => removeVar(i)}
                    class="text-danger hover:opacity-70 text-xs leading-none"
                    aria-label="Remove"
                  >&#x2715;</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="text-xs text-text-muted mb-3">No environment variables. Add one below.</p>
    {/if}

    <div class="flex gap-2 mt-2">
      <Button variant="secondary" size="sm" onclick={addVar}>Add Variable</Button>
      <Button size="sm" onclick={save} disabled={saving}>{saving ? 'Saving...' : 'Save'}</Button>
    </div>
  {/if}
</div>
