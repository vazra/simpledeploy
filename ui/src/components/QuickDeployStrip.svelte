<script>
  let { templates = [], featuredIds = [], onselect = () => {} } = $props()

  let featured = $derived.by(() => {
    const byId = new Map(templates.map((t) => [t.id, t]))
    const out = []
    for (const id of featuredIds) {
      const t = byId.get(id)
      if (t) out.push(t)
      if (out.length >= 6) break
    }
    return out
  })
</script>

{#if featured.length > 0}
  <div>
    <h3 class="text-sm font-medium text-text-primary mb-3">Quick deploy</h3>
    <p class="text-xs text-text-muted mb-3">One-click stacks with TLS, backups, and alerts preconfigured.</p>
    <div class="grid grid-cols-2 sm:grid-cols-3 gap-3">
      {#each featured as template}
        <button
          type="button"
          onclick={() => onselect(template.id)}
          class="bg-surface-3/50 border border-border/30 rounded-lg px-4 py-3 cursor-pointer hover:border-accent/50 transition-colors flex items-center gap-3 min-w-[220px] text-left"
        >
          <span class="text-2xl shrink-0">{template.icon}</span>
          <span class="flex flex-col min-w-0">
            <span class="font-medium text-sm text-text-primary truncate">{template.name}</span>
            <span class="text-xs text-text-muted line-clamp-1">{template.description}</span>
          </span>
        </button>
      {/each}
    </div>
  </div>
{/if}
