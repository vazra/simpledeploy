<script>
  let { title = '', open = false, onclose = () => {}, children } = $props()
</script>

{#if open}
  <div class="fixed inset-0 z-40" role="dialog" aria-modal="true">
    <button class="absolute inset-0 bg-black/50" onclick={onclose} aria-label="Close panel"></button>
    <div class="absolute right-0 top-0 h-full w-full max-w-md bg-surface-2 border-l border-border shadow-xl flex flex-col animate-slide-panel">
      <div class="flex items-center justify-between px-5 py-4 border-b border-border">
        <h3 class="text-lg font-semibold text-text-primary">{title}</h3>
        <button onclick={onclose} class="text-text-secondary hover:text-text-primary" aria-label="Close panel">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <div class="flex-1 overflow-y-auto px-5 py-4">
        {@render children()}
      </div>
    </div>
  </div>
{/if}
