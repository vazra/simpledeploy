<script>
  let { type = 'card', count = 1 } = $props()
</script>

{#each Array(count) as _, i}
  {#if type === 'card'}
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/30">
      <div class="h-3 bg-surface-3 rounded animate-shimmer w-1/3 mb-3"></div>
      <div class="h-6 bg-surface-3 rounded animate-shimmer w-1/2 mb-2"></div>
      <div class="h-3 bg-surface-3 rounded animate-shimmer w-2/3"></div>
    </div>
  {:else if type === 'table-row'}
    <div class="flex gap-4 py-3">
      <div class="h-4 bg-surface-3 rounded animate-shimmer w-1/4"></div>
      <div class="h-4 bg-surface-3 rounded animate-shimmer w-1/3"></div>
      <div class="h-4 bg-surface-3 rounded animate-shimmer w-1/6"></div>
    </div>
  {:else if type === 'chart'}
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/30">
      <div class="h-3 bg-surface-3 rounded animate-shimmer w-1/4 mb-4"></div>
      <div class="h-44 bg-surface-3 rounded animate-shimmer"></div>
    </div>
  {:else if type === 'line'}
    <div class="h-4 rounded w-full animate-shimmer"></div>
  {/if}
{/each}
