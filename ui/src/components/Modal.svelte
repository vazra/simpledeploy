<script>
  let { title = 'Confirm', message = '', onConfirm = () => {}, onCancel = () => {} } = $props()
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
  <div class="absolute inset-0 bg-black/60" onclick={onCancel}></div>
  <div class="relative bg-surface-2 border border-border rounded-lg p-6 min-w-80 max-w-md shadow-xl">
    <h3 class="text-base font-semibold text-text-primary mb-2">{title}</h3>
    <p class="text-sm text-text-secondary mb-5">{message}</p>
    <div class="flex justify-end gap-2">
      <button onclick={onCancel} class="px-3 py-1.5 text-sm border border-border rounded-md text-text-secondary hover:text-text-primary hover:border-text-secondary transition-colors">Cancel</button>
      <button onclick={onConfirm} class="px-3 py-1.5 text-sm bg-btn-danger text-white rounded-md hover:bg-btn-danger/80 transition-colors">Confirm</button>
    </div>
  </div>
</div>
