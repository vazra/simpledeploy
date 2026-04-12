<script>
  import SparklineChart from './SparklineChart.svelte'

  let { label, value, sub = '', icon = '', color = '', sparkline = [], sparklineColor = '#3b82f6', onclick = null } = $props()
</script>

{#if onclick}
  <button
    {onclick}
    class="flex flex-col gap-1.5 bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50 hover:shadow-md transition-all cursor-pointer text-left w-full"
  >
    <div class="flex items-center justify-between">
      <span class="text-xs font-medium text-text-muted tracking-wide">{label}</span>
      {#if icon}
        <span class="text-text-muted">{@html icon}</span>
      {/if}
    </div>
    <span class="text-2xl font-semibold tracking-tight {color || 'text-text-primary'}">{value}</span>
    {#if sparkline.length > 0}
      <SparklineChart data={sparkline} color={sparklineColor} />
    {/if}
    {#if sub}
      <span class="text-sm text-text-muted mt-0.5">{sub}</span>
    {/if}
  </button>
{:else}
  <div class="flex flex-col gap-1.5 bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50 hover:shadow-md transition-all">
    <div class="flex items-center justify-between">
      <span class="text-xs font-medium text-text-muted tracking-wide">{label}</span>
      {#if icon}
        <span class="text-text-muted">{@html icon}</span>
      {/if}
    </div>
    <span class="text-2xl font-semibold tracking-tight {color || 'text-text-primary'}">{value}</span>
    {#if sparkline.length > 0}
      <SparklineChart data={sparkline} color={sparklineColor} />
    {/if}
    {#if sub}
      <span class="text-sm text-text-muted mt-0.5">{sub}</span>
    {/if}
  </div>
{/if}
