<script>
  let { title = 'Confirm', message = '', onConfirm = () => {}, onCancel = () => {} } = $props()

  function onKeydown(e) {
    if (e.key === 'Escape') onCancel()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
  <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={onCancel} aria-label="Close"></button>
  <div class="relative bg-surface-2 border border-border/50 rounded-2xl p-6 min-w-80 max-w-md shadow-2xl animate-scale-in">
    <h3 class="text-lg font-semibold text-text-primary tracking-tight mb-2">{title}</h3>
    <p class="text-sm text-text-secondary mb-5">{message}</p>
    <div class="flex justify-end gap-2">
      <button onclick={onCancel} class="px-4 py-2 text-sm border border-border/50 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors">Cancel</button>
      <button onclick={onConfirm} class="px-4 py-2 text-sm bg-btn-danger text-white rounded-lg hover:bg-btn-danger/90 transition-colors shadow-sm">Confirm</button>
    </div>
  </div>
</div>
