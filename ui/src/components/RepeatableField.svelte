<script>
  let { label = '', hint = '', rows = [], fields = [], onchange = () => {} } = $props()

  // Local display rows include empties; we only emit non-empty to parent
  let localRows = $state([])
  let lastPropJson = $state('')

  // Sync from prop when external data changes (not from our own edits)
  $effect(() => {
    const json = JSON.stringify(rows)
    if (json !== lastPropJson) {
      lastPropJson = json
      // Preserve any trailing empty rows we added locally
      const emptyTrailing = localRows.filter(r => fields.every(f => !r[f.key]))
      localRows = [...rows, ...emptyTrailing]
    }
  })

  function addRow() {
    const empty = {}
    for (const f of fields) empty[f.key] = ''
    localRows = [...localRows, empty]
  }

  function removeRow(i) {
    localRows = localRows.filter((_, idx) => idx !== i)
    emit()
  }

  function updateRow(i, key, value) {
    localRows = localRows.map((r, idx) => idx === i ? { ...r, [key]: value } : r)
    emit()
  }

  function emit() {
    const nonEmpty = localRows.filter(r => fields.some(f => r[f.key]))
    lastPropJson = JSON.stringify(nonEmpty)
    onchange(nonEmpty)
  }
</script>

<div class="space-y-2">
  {#if label}
    <div>
      <span class="text-xs font-medium text-text-primary">{label}</span>
      {#if hint}
        <span class="text-xs text-text-muted ml-1.5">{hint}</span>
      {/if}
    </div>
  {/if}

  {#each localRows as row, i}
    <div class="flex items-center gap-2">
      {#each fields as field}
        <input
          type={field.type ?? 'text'}
          placeholder={field.placeholder ?? ''}
          value={row[field.key] ?? ''}
          oninput={(e) => updateRow(i, field.key, e.currentTarget.value)}
          class="flex-1 bg-input-bg border border-border rounded px-2.5 py-1.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50 min-w-0"
        />
      {/each}
      <button
        type="button"
        onclick={() => removeRow(i)}
        class="flex-shrink-0 p-1.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
        aria-label="Remove row"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
        </svg>
      </button>
    </div>
  {/each}

  <button
    type="button"
    onclick={addRow}
    class="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary hover:bg-surface-3 px-2 py-1 rounded transition-colors"
  >
    <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
    </svg>
    Add
  </button>
</div>
