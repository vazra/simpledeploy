<script>
  let { value = '', onchange = () => {}, error = '', minHeight = '400px' } = $props()

  let textareaEl = $state(null)
  let gutterEl = $state(null)

  let lineCount = $derived(value ? value.split('\n').length : 1)
  let lineNumbers = $derived(Array.from({ length: lineCount }, (_, i) => i + 1))

  function handleInput(e) {
    onchange(e.currentTarget.value)
  }

  function handleKeydown(e) {
    if (e.key === 'Tab') {
      e.preventDefault()
      const el = e.currentTarget
      const start = el.selectionStart
      const end = el.selectionEnd
      const newVal = el.value.substring(0, start) + '  ' + el.value.substring(end)
      onchange(newVal)
      // restore cursor after Svelte re-renders
      requestAnimationFrame(() => {
        el.selectionStart = start + 2
        el.selectionEnd = start + 2
      })
    }
  }

  function syncScroll(e) {
    if (gutterEl) gutterEl.scrollTop = e.currentTarget.scrollTop
  }
</script>

<div class="bg-surface-1 border border-border rounded-lg overflow-hidden">
  {#if error}
    <div class="bg-danger/10 text-danger text-xs px-3 py-2 border-b border-danger/20">
      {error}
    </div>
  {/if}
  <div class="flex" style="min-height: {minHeight};">
    <div
      bind:this={gutterEl}
      class="bg-surface-2 text-text-muted text-xs font-mono select-none overflow-hidden flex-shrink-0 py-2 px-2 text-right"
      style="min-width: 3rem;"
      aria-hidden="true"
    >
      {#each lineNumbers as n}
        <div class="leading-5">{n}</div>
      {/each}
    </div>
    <textarea
      bind:this={textareaEl}
      {value}
      oninput={handleInput}
      onkeydown={handleKeydown}
      onscroll={syncScroll}
      spellcheck="false"
      autocomplete="off"
      autocorrect="off"
      autocapitalize="off"
      class="flex-1 bg-transparent text-text-primary text-sm font-mono resize-none border-none outline-none p-2 leading-5 overflow-y-auto"
      style="min-height: {minHeight};"
    ></textarea>
  </div>
</div>
