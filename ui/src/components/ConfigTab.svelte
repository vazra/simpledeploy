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

  <div class="flex justify-end mt-4 pt-3 border-t border-border/30">
    <Button onclick={handleSave} loading={saving} disabled={mode === 'visual' && hasValidationErrors}>Save &amp; Deploy</Button>
  </div>

  {#if versions.length > 0}
    <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden mt-4">
      <button
        type="button"
        onclick={() => showHistory = !showHistory}
        class="w-full flex items-center justify-between px-5 py-3 text-left hover:bg-surface-hover transition-colors"
      >
        <span class="text-xs font-medium text-text-primary">Deploy History ({versions.length})</span>
        <svg
          class="w-3.5 h-3.5 text-text-muted transition-transform {showHistory ? 'rotate-180' : ''}"
          fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
        ><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
      </button>
      {#if showHistory}
        <div class="overflow-x-auto border-t border-border/30">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-2 px-4">Version</th>
              <th class="text-left text-xs font-medium text-text-muted py-2 px-4">Hash</th>
              <th class="text-left text-xs font-medium text-text-muted py-2 px-4">Deployed</th>
              <th class="py-2 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each versions as v}
                <tr class="hover:bg-surface-hover">
                  <td class="py-2 px-4 text-xs">v{v.version}</td>
                  <td class="py-2 px-4 font-mono text-xs">{v.hash?.slice(0, 8)}</td>
                  <td class="py-2 px-4 text-xs">{v.created_at ? new Date(v.created_at).toLocaleString() : '-'}</td>
                  <td class="py-2 px-4 flex items-center gap-1">
                    <Button variant="ghost" size="sm" onclick={() => rollbackTarget = v.id}>Rollback</Button>
                    <button
                      type="button"
                      onclick={async () => { await api.deleteVersion(slug, v.id); loadHistory() }}
                      class="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
                      title="Delete version"
                    >
                      <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
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
