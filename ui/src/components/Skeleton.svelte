<script>
  let { type = 'card', count = 1 } = $props()
</script>

{#each Array(count) as _, i}
  {#if type === 'card'}
    <div class="bg-surface-2 border border-border rounded-lg p-4 animate-pulse">
      <div class="h-3 bg-surface-3 rounded w-1/3 mb-3"></div>
      <div class="h-6 bg-surface-3 rounded w-1/2 mb-2"></div>
      <div class="h-3 bg-surface-3 rounded w-2/3"></div>
    </div>
  {:else if type === 'table-row'}
    <div class="flex gap-4 py-3 animate-pulse">
      <div class="h-4 bg-surface-3 rounded w-1/4"></div>
      <div class="h-4 bg-surface-3 rounded w-1/3"></div>
      <div class="h-4 bg-surface-3 rounded w-1/6"></div>
    </div>
  {:else if type === 'chart'}
    <div class="bg-surface-2 border border-border rounded-lg p-4 animate-pulse">
      <div class="h-3 bg-surface-3 rounded w-1/4 mb-4"></div>
      <div class="h-44 bg-surface-3 rounded"></div>
    </div>
  {:else if type === 'line'}
    <div class="h-4 bg-surface-3 rounded w-full animate-pulse"></div>
  {/if}
{/each}
