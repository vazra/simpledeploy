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

  let { slug, onModeChange = () => {} } = $props()

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
    const res = await api.deploy(slug, encoded)
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
  <div class="flex gap-0.5 bg-surface-3/40 rounded-lg p-0.5 w-fit mb-4">
    <button
      class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors
        {mode === 'visual' ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
      onclick={() => switchMode('visual')}
    >Visual</button>
    <button
      class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors
        {mode === 'yaml' ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
      onclick={() => switchMode('yaml')}
    >YAML</button>
  </div>

  {#if mode === 'visual'}
    <VisualEditor {compose} {slug} onchange={(updated) => { compose = updated }} onerrors={(errs) => { hasValidationErrors = Object.keys(errs).length > 0 }} />
  {:else}
    <YamlEditor value={currentYaml} error={yamlError} onchange={(val) => {
      currentYaml = val
      try { yaml.load(val); yamlError = '' } catch (e) { yamlError = e.message }
    }} />

    <!-- .env file editor -->
    <div class="mt-4">
      <div class="flex items-center mb-2">
        <span class="text-xs font-medium text-text-primary">.env</span>
        <span class="text-xs text-text-muted ml-1.5">KEY=value, one per line</span>
      </div>
      <textarea
        class="w-full bg-input-bg border border-border/50 rounded-lg px-3 py-2.5 text-sm font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/60 resize-y min-h-20"
        rows="5"
        placeholder="DB_HOST=localhost&#10;DB_PORT=5432"
        bind:value={envText}
      ></textarea>
    </div>
  {/if}

  <div class="flex justify-end mt-4 pt-3 border-t border-border/30">
    <Button onclick={handleSave} loading={saving} disabled={mode === 'visual' && hasValidationErrors}>Save &amp; Deploy</Button>
  </div>

  {#if versions.length > 0}
    <button
      type="button"
      onclick={() => showHistory = !showHistory}
      class="w-full flex items-center justify-between mt-4 px-1 py-2 text-left"
    >
      <span class="text-xs font-medium text-text-secondary">Deploy History ({versions.length})</span>
      <svg
        class="w-3.5 h-3.5 text-text-muted transition-transform {showHistory ? 'rotate-180' : ''}"
        fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
      ><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
    </button>
    {#if showHistory}
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead><tr class="border-b border-border/50">
            <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Version</th>
            <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Hash</th>
            <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Deployed</th>
            <th class="py-2 px-3"></th>
          </tr></thead>
          <tbody class="divide-y divide-border/30">
            {#each versions as v}
              <tr class="hover:bg-surface-hover">
                <td class="py-2 px-3 text-xs">v{v.version}</td>
                <td class="py-2 px-3 font-mono text-xs">{v.hash?.slice(0, 8)}</td>
                <td class="py-2 px-3 text-xs">{v.created_at ? new Date(v.created_at).toLocaleString() : '-'}</td>
                <td class="py-2 px-3">
                  <Button variant="ghost" size="sm" onclick={() => rollbackTarget = v.id}>Rollback</Button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}

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
