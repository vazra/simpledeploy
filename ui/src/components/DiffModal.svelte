<script>
  import { diffLines } from 'diff'

  let { oldText = '', newText = '', onConfirm = () => {}, onCancel = () => {} } = $props()

  let parts = $derived(diffLines(oldText, newText))

  function onKeydown(e) {
    if (e.key === 'Escape') onCancel()
  }

  function getLines(value) {
    return value.replace(/\n$/, '').split('\n')
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
  <button class="absolute inset-0 bg-black/60" onclick={onCancel} aria-label="Close"></button>
  <div class="relative bg-surface-2 border border-border rounded-lg p-6 w-full max-w-3xl mx-4 shadow-xl flex flex-col" style="max-height: 90vh;">
    <h3 class="text-base font-semibold text-text-primary mb-4 flex-shrink-0">Review Changes</h3>

    <div class="flex-1 overflow-y-auto min-h-0 mb-5">
      <pre class="font-mono text-xs bg-surface-1 border border-border rounded-lg overflow-auto" style="max-height: 60vh;"><code
        >{#each parts as part}{@const lines = getLines(part.value)}{#if part.added}{#each lines as line}<span class="block bg-success/10 text-success px-2">+ {line}</span>
{/each}{:else if part.removed}{#each lines as line}<span class="block bg-danger/10 text-danger px-2">- {line}</span>
{/each}{:else}{#each lines as line, i}{#if i < 3 || lines.length - i <= 3}<span class="block text-text-secondary px-2">  {line}</span>
{:else if i === 3 && lines.length > 6}<span class="block text-text-muted px-2 italic">  ... {lines.length - 6} unchanged lines ...</span>
{/if}{/each}{/if}{/each}</code></pre>
    </div>

    <div class="flex justify-end gap-2 flex-shrink-0">
      <button
        type="button"
        onclick={onCancel}
        class="px-3.5 py-2 text-sm bg-surface-3 hover:bg-surface-3/80 text-text-primary border border-border rounded-md transition-colors"
      >
        Cancel
      </button>
      <button
        type="button"
        onclick={onConfirm}
        class="px-3.5 py-2 text-sm bg-btn-primary hover:bg-btn-primary-hover text-white rounded-md transition-colors"
      >
        Confirm &amp; Deploy
      </button>
    </div>
  </div>
</div>
