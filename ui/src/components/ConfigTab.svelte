<script>
  import { onMount } from 'svelte'
  import yaml from 'js-yaml'
  import { api } from '../lib/api.js'
  import { toasts } from '../lib/stores/toast.js'
  import VisualEditor from './VisualEditor.svelte'
  import YamlEditor from './YamlEditor.svelte'
  import DiffModal from './DiffModal.svelte'
  import Button from './Button.svelte'
  import Skeleton from './Skeleton.svelte'
  import Modal from './Modal.svelte'

  let { slug, composePath = '', onModeChange = () => {} } = $props()

  let envPath = $derived(composePath ? composePath.replace(/[^/]+$/, '.env') : '.env')

  let mode = $state('visual')
  let originalYaml = $state('')
  let currentYaml = $state('')
  let compose = $state({})
  let yamlError = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let showDiff = $state(false)
  let hasValidationErrors = $state(false)

  // .env plain text (shown in YAML mode)
  let envText = $state('')
  let envOriginal = $state('')
  let savingEnv = $state(false)

  function envToText(vars) {
    return vars.map(v => `${v.key}=${v.value}`).join('\n')
  }

  function textToEnv(text) {
    return text.split('\n').filter(l => l.trim() && !l.startsWith('#')).map(l => {
      const idx = l.indexOf('=')
      if (idx === -1) return { key: l.trim(), value: '' }
      return { key: l.slice(0, idx).trim(), value: l.slice(idx + 1) }
    })
  }

  async function loadEnv() {
    if (!slug) return
    try {
      const res = await api.getEnv(slug)
      const t = envToText(res.data || [])
      envText = t
      envOriginal = t
    } catch { /* no env file */ }
  }

  async function saveEnv() {
    savingEnv = true
    await api.putEnv(slug, textToEnv(envText))
    envOriginal = envText
    savingEnv = false
  }

  let versions = $state([])
  let showHistory = $state(false)
  let rollbackTarget = $state(null)
  let rollingBack = $state(false)
  let restoreTarget = $state(null)
  let restoring = $state(false)

  // Inline editing state
  let editingId = $state(null)
  let editName = $state('')
  let editNotes = $state('')
  let savingEdit = $state(false)

  function relativeTime(isoStr) {
    if (!isoStr) return ''
    const diff = Date.now() - new Date(isoStr).getTime()
    const secs = Math.floor(diff / 1000)
    if (secs < 60) return 'just now'
    const mins = Math.floor(secs / 60)
    if (mins < 60) return `${mins}m ago`
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) return `${hrs}h ago`
    const days = Math.floor(hrs / 24)
    if (days < 30) return `${days}d ago`
    const months = Math.floor(days / 30)
    return `${months}mo ago`
  }

  function startEditing(v) {
    editingId = v.id
    editName = v.name || ''
    editNotes = v.notes || ''
  }

  function cancelEditing() {
    editingId = null
    editName = ''
    editNotes = ''
  }

  async function saveEditing() {
    if (!editingId) return
    savingEdit = true
    await api.updateComposeVersion(slug, editingId, { name: editName, notes: editNotes })
    savingEdit = false
    editingId = null
    loadHistory()
  }

  async function handleRestore() {
    if (!restoreTarget) return
    restoring = true
    const res = await api.restoreComposeVersion(slug, restoreTarget)
    restoring = false
    restoreTarget = null
    if (!res.error) {
      const compRes = await api.getCompose(slug)
      if (!compRes.error) {
        originalYaml = normalizeYaml(compRes.data)
        currentYaml = compRes.data
        try { compose = yaml.load(compRes.data) || {} } catch {}
      }
      loadHistory()
    }
  }
  function normalizeYaml(str) {
    try { return yaml.dump(yaml.load(str), { lineWidth: -1 }) } catch { return str }
  }

  function encodeBase64(str) {
    return btoa(String.fromCodePoint(...new TextEncoder().encode(str)))
  }

  async function loadCompose() {
    const res = await api.getCompose(slug)
    if (res.error) { toasts.error('Failed to load compose'); return }
    originalYaml = normalizeYaml(res.data)
    currentYaml = res.data
    try {
      compose = yaml.load(res.data) || {}
    } catch (e) {
      yamlError = e.message
    }
    loading = false
  }

  export function reload() {
    loadEnv()
    return loadCompose()
  }

  onMount(() => {
    loadCompose()
    loadHistory()
    loadEnv()
  })

  export function switchToMode(newMode) { switchMode(newMode) }

  function switchMode(newMode) {
    if (newMode === mode) return

    if (newMode === 'yaml') {
      try {
        currentYaml = yaml.dump(compose, { lineWidth: -1 })
        yamlError = ''
      } catch (e) {
        yamlError = e.message
      }
    } else {
      try {
        compose = yaml.load(currentYaml) || {}
        yamlError = ''
      } catch (e) {
        toasts.error('Cannot switch to Visual mode: YAML has syntax errors')
        return
      }
    }
    mode = newMode
    onModeChange(newMode)
  }

  function handleExport() {
    let yamlStr
    if (mode === 'visual') {
      try {
        yamlStr = yaml.dump(compose, { lineWidth: -1 })
      } catch {
        toasts.error('Failed to serialize compose')
        return
      }
    } else {
      yamlStr = currentYaml
    }
    const blob = new Blob([yamlStr], { type: 'text/yaml' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${slug}-docker-compose.yml`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  async function handleSave() {
    let yamlStr
    if (mode === 'visual') {
      try {
        yamlStr = yaml.dump(compose, { lineWidth: -1 })
      } catch (e) {
        toasts.error('Failed to serialize compose')
        return
      }
    } else {
      yamlStr = currentYaml
      try { yaml.load(yamlStr) } catch (e) {
        toasts.error('Fix YAML errors before saving')
        return
      }
    }

    if (normalizeYaml(yamlStr) === originalYaml) {
      toasts.info('No changes to deploy')
      return
    }

    currentYaml = yamlStr
    showDiff = true
  }

  async function confirmDeploy() {
    saving = true
    if (mode === 'yaml' && envText !== envOriginal) {
      await saveEnv()
    }
    const encoded = encodeBase64(currentYaml)
    const res = await api.deploy(slug, encoded, 'update', true)
    saving = false
    showDiff = false
    if (!res.error) {
      originalYaml = normalizeYaml(currentYaml)
      loadHistory()
    }
  }

  async function loadHistory() {
    const vRes = await api.getComposeVersions(slug)
    versions = vRes.data || []
  }

  async function handleRollback() {
    if (!rollbackTarget) return
    rollingBack = true
    const res = await api.rollbackApp(slug, rollbackTarget)
    rollingBack = false
    rollbackTarget = null
    if (!res.error) {
      const compRes = await api.getCompose(slug)
      if (!compRes.error) {
        originalYaml = normalizeYaml(compRes.data)
        currentYaml = compRes.data
        try { compose = yaml.load(compRes.data) || {} } catch {}
      }
      loadHistory()
    }
  }
</script>

{#if loading}
  <div class="space-y-3">
    <Skeleton type="card" count={3} />
  </div>
{:else}
  {#if mode === 'visual'}
    <VisualEditor {compose} {slug} onchange={(updated) => { compose = updated }} onerrors={(errs) => { hasValidationErrors = Object.keys(errs).length > 0 }} />
  {:else}
    <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden">
      <div class="px-4 py-2 border-b border-border/30 bg-surface-3/30">
        <span class="text-xs font-mono text-text-secondary">{composePath || 'docker-compose.yml'}</span>
      </div>
      <YamlEditor bordered={false} value={currentYaml} error={yamlError} onchange={(val) => {
        currentYaml = val
        try { yaml.load(val); yamlError = '' } catch (e) { yamlError = e.message }
      }} />
    </div>

    <!-- .env file editor -->
    <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden mt-4">
      <div class="px-4 py-2 border-b border-border/30 bg-surface-3/30">
        <span class="text-xs font-mono text-text-secondary">{envPath}</span>
      </div>
      <YamlEditor bordered={false} value={envText} onchange={(val) => { envText = val }} minHeight="120px" />
    </div>
  {/if}

  <div class="flex justify-end gap-2 mt-4 pt-3 border-t border-border/30">
    <Button variant="ghost" onclick={handleExport} title="Download compose YAML">Export</Button>
    <Button onclick={handleSave} loading={saving} disabled={mode === 'visual' && hasValidationErrors}>Save &amp; Deploy</Button>
  </div>

  <!-- Deploy History -->
  <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden mt-4">
    <button
      type="button"
      onclick={() => showHistory = !showHistory}
      class="w-full flex items-center justify-between px-5 py-3 text-left hover:bg-surface-hover transition-colors"
    >
      <span class="text-xs font-medium text-text-primary">Deploy History {versions.length ? `(${versions.length})` : ''}</span>
      <svg
        class="w-3.5 h-3.5 text-text-muted transition-transform {showHistory ? 'rotate-180' : ''}"
        fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
      ><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
    </button>
    {#if showHistory}
      <div class="border-t border-border/30 px-5 py-4">
        {#if versions.length === 0}
          <p class="text-sm text-text-muted text-center py-4">No version history yet.</p>
        {:else}
          <div class="space-y-3">
            {#each versions as v, i}
              <div class="relative pl-6 pb-3 {i < versions.length - 1 ? 'border-l-2 border-border/30 ml-1.5' : 'ml-1.5'}">
                <!-- Timeline dot -->
                <div class="absolute -left-[5px] top-1 w-3 h-3 rounded-full {i === 0 ? 'bg-accent ring-2 ring-accent/20' : 'bg-surface-3 border-2 border-border/50'}"></div>

                <div class="bg-surface-1 rounded-lg border border-border/30 p-3 group">
                  <!-- Header row -->
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-xs font-semibold text-text-primary">v{v.version}</span>
                    {#if i === 0}
                      <span class="text-[10px] px-1.5 py-0.5 rounded bg-accent/10 text-accent font-medium">current</span>
                    {/if}
                    {#if v.hash}
                      <span class="text-[11px] font-mono text-text-muted">{v.hash.slice(0, 8)}</span>
                    {/if}
                    <span class="text-[11px] text-text-muted ml-auto" title={v.created_at ? new Date(v.created_at).toLocaleString() : ''}>
                      {relativeTime(v.created_at)}
                    </span>
                  </div>

                  <!-- Name & Notes display/edit -->
                  {#if editingId === v.id}
                    <div class="mt-2 space-y-2">
                      <input
                        type="text"
                        bind:value={editName}
                        placeholder="Version name (e.g. 'Added redis cache')"
                        class="w-full bg-surface-3 border border-border/50 rounded-lg px-2.5 py-1.5 text-xs text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40"
                        onkeydown={(e) => { if (e.key === 'Enter') saveEditing(); if (e.key === 'Escape') cancelEditing() }}
                      />
                      <textarea
                        bind:value={editNotes}
                        placeholder="Notes (optional)"
                        rows="2"
                        class="w-full bg-surface-3 border border-border/50 rounded-lg px-2.5 py-1.5 text-xs text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40 resize-y"
                        onkeydown={(e) => { if (e.key === 'Escape') cancelEditing() }}
                      ></textarea>
                      <div class="flex justify-end gap-1.5">
                        <Button variant="ghost" size="sm" onclick={cancelEditing}>Cancel</Button>
                        <Button size="sm" onclick={saveEditing} loading={savingEdit}>Save</Button>
                      </div>
                    </div>
                  {:else}
                    {#if v.name || v.notes}
                      <div class="mt-1.5">
                        {#if v.name}
                          <p class="text-xs text-text-primary font-medium">{v.name}</p>
                        {/if}
                        {#if v.notes}
                          <p class="text-xs text-text-secondary mt-0.5">{v.notes}</p>
                        {/if}
                      </div>
                    {/if}

                    <!-- Actions row -->
                    <div class="flex items-center gap-1 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        type="button"
                        onclick={() => startEditing(v)}
                        class="px-2 py-1 text-[11px] rounded text-text-muted hover:text-accent hover:bg-accent/10 transition-colors"
                        title={v.name || v.notes ? 'Edit name & notes' : 'Add name & notes'}
                      >
                        <svg class="w-3 h-3 inline mr-0.5 -mt-px" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" /></svg>
                        {v.name || v.notes ? 'Edit' : 'Label'}
                      </button>
                      <a
                        href={api.downloadComposeVersionUrl(slug, v.id)}
                        download
                        class="px-2 py-1 text-[11px] rounded text-text-muted hover:text-accent hover:bg-accent/10 transition-colors"
                        title="Download compose file"
                      >
                        <svg class="w-3 h-3 inline mr-0.5 -mt-px" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>
                        Download
                      </a>
                      {#if i > 0}
                        <button
                          type="button"
                          onclick={() => restoreTarget = v.id}
                          class="px-2 py-1 text-[11px] rounded text-text-muted hover:text-warning hover:bg-warning/10 transition-colors"
                          title="Restore this version"
                        >
                          <svg class="w-3 h-3 inline mr-0.5 -mt-px" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" /></svg>
                          Restore
                        </button>
                      {/if}
                      <button
                        type="button"
                        onclick={async () => { await api.deleteVersion(slug, v.id); loadHistory() }}
                        class="px-2 py-1 text-[11px] rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors ml-auto"
                        title="Delete version"
                      >
                        <svg class="w-3 h-3 inline mr-0.5 -mt-px" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                      </button>
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

{/if}

{#if showDiff}
  <DiffModal
    oldText={originalYaml}
    newText={currentYaml}
    onConfirm={confirmDeploy}
    onCancel={() => showDiff = false}
  />
{/if}

{#if rollbackTarget}
  <Modal
    title="Confirm Rollback"
    message="This will restore a previous compose version and redeploy. Continue?"
    onConfirm={handleRollback}
    onCancel={() => rollbackTarget = null}
  />
{/if}

{#if restoreTarget}
  <Modal
    title="Restore Version"
    message="This will redeploy using this compose version. Your current compose file will be replaced. Continue?"
    onConfirm={handleRestore}
    onCancel={() => restoreTarget = null}
  />
{/if}
