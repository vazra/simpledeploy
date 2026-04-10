<script>
  let { label, value, sub = '', icon = '', color = '' } = $props()
</script>

<div class="flex flex-col gap-1 bg-surface-2 border border-border rounded-lg p-4">
  <div class="flex items-center justify-between">
    <span class="text-xs font-medium text-text-secondary uppercase tracking-wider">{label}</span>
    {#if icon}
      <span class="text-text-muted">{@html icon}</span>
    {/if}
  </div>
  <span class="text-2xl font-semibold {color || 'text-text-primary'}">{value}</span>
  {#if sub}
    <span class="text-xs text-text-secondary">{sub}</span>
  {/if}
</div>
