<script>
  let { open = false, title = '', onclose = () => {}, children } = $props()

  function onKeydown(e) {
    if (open && e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4" role="dialog" aria-modal="true">
    <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={onclose} aria-label="Close"></button>
    <div class="relative bg-surface-2 border border-border/50 rounded-2xl shadow-2xl animate-scale-in max-w-2xl w-full max-h-[80vh] flex flex-col">
      <div class="flex items-center justify-between px-6 py-4 border-b border-border/30 shrink-0">
        <h3 class="text-lg font-semibold text-text-primary tracking-tight">{title}</h3>
        <button onclick={onclose} class="text-text-muted hover:text-text-primary transition-colors p-1 rounded-lg hover:bg-surface-3" aria-label="Close">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <div class="overflow-y-auto p-6">
        {@render children()}
      </div>
    </div>
  </div>
{/if}
