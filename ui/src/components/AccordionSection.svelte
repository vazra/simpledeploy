<script>
  let { title, expanded = false, children } = $props()
</script>

<div class="bg-surface-2 border border-border rounded-lg overflow-hidden">
  <button
    type="button"
    class="w-full flex items-center justify-between px-4 py-3 text-left hover:bg-surface-3/50 transition-colors"
    onclick={() => (expanded = !expanded)}
  >
    <span class="text-sm font-semibold text-text-primary">{title}</span>
    <svg
      class="w-4 h-4 text-text-muted transition-transform duration-200 {expanded ? 'rotate-180' : ''}"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="2"
    >
      <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
    </svg>
  </button>
  {#if expanded}
    <div class="px-4 pb-4 pt-1">
      {@render children()}
    </div>
  {/if}
</div>
