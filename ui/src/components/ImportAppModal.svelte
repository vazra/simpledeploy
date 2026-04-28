<script>
  import Button from './Button.svelte'
  import { api } from '../lib/api.js'

  let { open = false, onclose = () => {}, onImported = () => {} } = $props()

  let file = $state(null)
  let fileName = $state('')
  let fileSize = $state(0)
  let mode = $state('new') // 'new' | 'overwrite'
  let slug = $state('')
  let importing = $state(false)
  let errorMsg = $state('')
  let preview = $state(null) // overwrite preview response

  function reset() {
    file = null
    fileName = ''
    fileSize = 0
    mode = 'new'
    slug = ''
    importing = false
    errorMsg = ''
    preview = null
  }

  function handleFileChange(e) {
    const f = e.target.files?.[0]
    if (!f) return
    file = f
    fileName = f.name
    fileSize = f.size
    errorMsg = ''
  }

  function formatSize(n) {
    if (n < 1024) return `${n} B`
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
    return `${(n / 1024 / 1024).toFixed(2)} MB`
  }

  async function doImport() {
    const res = await api.importApp(file, { mode, slug: slug.trim() })
    importing = false
    if (res.error) {
      errorMsg = res.error
      return
    }
    const newSlug = res.data?.slug || slug.trim()
    reset()
    onImported({ slug: newSlug, mode })
    onclose()
  }

  async function handleImport() {
    if (!file || !slug.trim()) return
    importing = true
    errorMsg = ''
    if (mode === 'overwrite') {
      const res = await api.importAppPreview(file, { mode, slug: slug.trim() })
      importing = false
      if (res.error) {
        errorMsg = res.error
        return
      }
      preview = res.data
      return
    }
    await doImport()
  }

  async function handleConfirm() {
    importing = true
    errorMsg = ''
    await doImport()
  }

  function handleBack() {
    if (importing) return
    preview = null
  }

  function handleClose() {
    if (importing) return
    reset()
    onclose()
  }

  function onKeydown(e) {
    if (open && e.key === 'Escape') handleClose()
  }

  let canImport = $derived(!!file && !!slug.trim() && !importing)

  function formatDelta(n) {
    if (n > 0) return `+${n}`
    return `${n}`
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
<div class="fixed inset-0 z-50 flex items-center justify-center p-4" role="dialog" aria-modal="true">
  <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={handleClose} aria-label="Close"></button>
  <div class="relative bg-surface-2 border border-border/50 rounded-2xl shadow-2xl animate-scale-in max-w-lg w-full max-h-[85vh] flex flex-col">
    <div class="flex items-center justify-between px-6 py-4 border-b border-border/30 shrink-0">
      <h3 class="text-lg font-semibold text-text-primary tracking-tight">
        {preview ? 'Confirm overwrite' : 'Import App'}
      </h3>
      <button onclick={handleClose} class="text-text-muted hover:text-text-primary transition-colors p-1 rounded-lg hover:bg-surface-3" aria-label="Close">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>

    {#if preview}
      <div class="overflow-y-auto p-6 flex flex-col gap-4" data-testid="import-preview">
        <p class="text-sm text-text-primary">
          Overwriting app <span class="font-mono font-semibold">{preview.slug}</span>
        </p>
        <div class="flex flex-col gap-2 text-sm">
          <div class="flex items-center justify-between bg-surface-3/50 rounded-md px-3 py-2">
            <span class="text-text-secondary">Compose file</span>
            <span class={preview.changes?.compose_changed ? 'text-warning font-medium' : 'text-text-muted'}>
              {preview.changes?.compose_changed ? 'Changed' : 'Unchanged'}
            </span>
          </div>
          <div class="flex items-center justify-between bg-surface-3/50 rounded-md px-3 py-2">
            <span class="text-text-secondary">Sidecar config</span>
            <span class={preview.changes?.sidecar_changed ? 'text-warning font-medium' : 'text-text-muted'}>
              {preview.changes?.sidecar_changed ? 'Changed' : 'Unchanged'}
            </span>
          </div>
          <div class="flex items-center justify-between bg-surface-3/50 rounded-md px-3 py-2">
            <span class="text-text-secondary">Alert rules</span>
            <span class="font-mono text-xs text-text-primary">
              {preview.changes?.alert_rule_count_current ?? 0} -&gt; {preview.changes?.alert_rule_count_incoming ?? 0}
              <span class="ml-1 text-text-muted">({formatDelta(preview.changes?.alert_rule_count_delta ?? 0)})</span>
            </span>
          </div>
          <div class="flex items-center justify-between bg-surface-3/50 rounded-md px-3 py-2">
            <span class="text-text-secondary">Backup configs</span>
            <span class="font-mono text-xs text-text-primary">
              {preview.changes?.backup_config_count_current ?? 0} -&gt; {preview.changes?.backup_config_count_incoming ?? 0}
              <span class="ml-1 text-text-muted">({formatDelta(preview.changes?.backup_config_count_delta ?? 0)})</span>
            </span>
          </div>
        </div>
        <div class="bg-danger/10 border border-danger/30 rounded-md px-3 py-2 text-xs text-text-secondary">
          The on-disk <span class="font-mono">.env</span> and <span class="font-mono">simpledeploy.secrets.yml</span> will be preserved. The compose and sidecar will be replaced.
        </div>
        {#if errorMsg}
          <div data-testid="import-error" class="text-xs text-danger bg-danger/10 border border-danger/30 rounded-md px-3 py-2">
            {errorMsg}
          </div>
        {/if}
      </div>

      <div class="flex justify-end gap-2 px-6 py-4 border-t border-border/30 shrink-0">
        <Button variant="secondary" size="sm" onclick={handleBack} disabled={importing}>Back</Button>
        <Button variant="danger" size="sm" onclick={handleConfirm} disabled={importing} loading={importing}>Confirm overwrite</Button>
      </div>
    {:else}
      <div class="overflow-y-auto p-6 flex flex-col gap-5">
        <!-- File input -->
        <div>
          <label class="block text-sm font-medium text-text-primary mb-1.5" for="import-file">Bundle file</label>
          <input
            id="import-file"
            data-testid="import-file"
            type="file"
            accept=".zip,application/zip"
            onchange={handleFileChange}
            class="block w-full text-sm text-text-primary file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border-0 file:text-xs file:font-medium file:bg-surface-3 file:text-text-primary hover:file:bg-surface-hover"
          />
          {#if fileName}
            <p class="text-xs text-text-muted mt-1.5">
              <span class="font-mono text-text-primary">{fileName}</span>
              <span class="ml-1">({formatSize(fileSize)})</span>
            </p>
          {/if}
        </div>

        <div class="bg-info/10 border border-info/30 rounded-md px-3 py-2 text-xs text-text-secondary">
          Secrets and env values are not exported. You'll need to re-enter them after import.
        </div>

        <!-- Mode -->
        <div>
          <p class="block text-sm font-medium text-text-primary mb-2">Mode</p>
          <div class="flex flex-col gap-2">
            <label class="flex items-center gap-2 text-sm text-text-primary">
              <input type="radio" name="import-mode" value="new" checked={mode === 'new'} onchange={() => mode = 'new'} />
              Create new app
            </label>
            <label class="flex items-center gap-2 text-sm text-text-primary">
              <input type="radio" name="import-mode" value="overwrite" checked={mode === 'overwrite'} onchange={() => mode = 'overwrite'} />
              Overwrite existing app
            </label>
          </div>
        </div>

        <!-- Slug -->
        <div>
          <label class="block text-sm font-medium text-text-primary mb-1.5" for="import-slug">
            {mode === 'overwrite' ? 'Existing app slug to overwrite' : 'New app slug'}
          </label>
          <input
            id="import-slug"
            data-testid="import-slug"
            type="text"
            bind:value={slug}
            placeholder="my-app"
            class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30"
          />
        </div>

        {#if errorMsg}
          <div data-testid="import-error" class="text-xs text-danger bg-danger/10 border border-danger/30 rounded-md px-3 py-2">
            {errorMsg}
          </div>
        {/if}
      </div>

      <div class="flex justify-end gap-2 px-6 py-4 border-t border-border/30 shrink-0">
        <Button variant="secondary" size="sm" onclick={handleClose} disabled={importing}>Cancel</Button>
        <Button size="sm" onclick={handleImport} disabled={!canImport} loading={importing}>Import</Button>
      </div>
    {/if}
  </div>
</div>
{/if}
