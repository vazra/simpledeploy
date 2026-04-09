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
  import Badge from './Badge.svelte'

  let { slug } = $props()

  let mode = $state('visual')
  let originalYaml = $state('')
  let currentYaml = $state('')
  let compose = $state({})
  let yamlError = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let showDiff = $state(false)
  let hasValidationErrors = $state(false)

  let versions = $state([])
  let events = $state([])
  let rollbackTarget = $state(null)
  let rollingBack = $state(false)

  function normalizeYaml(str) {
    try { return yaml.dump(yaml.load(str), { lineWidth: -1 }) } catch { return str }
  }

  function encodeBase64(str) {
    return btoa(String.fromCodePoint(...new TextEncoder().encode(str)))
  }

  onMount(async () => {
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
    loadHistory()
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
    const [vRes, eRes] = await Promise.all([
      api.getComposeVersions(slug),
      api.getDeployEvents(slug),
    ])
    versions = vRes.data || []
    events = eRes.data || []
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
  <div class="flex gap-1 bg-surface-1 rounded-lg p-1 w-fit mb-4">
    <button
      class="px-3 py-1.5 text-xs rounded-md transition-colors
        {mode === 'visual' ? 'bg-surface-3 text-text-primary' : 'text-text-secondary hover:text-text-primary'}"
      onclick={() => switchMode('visual')}
    >Visual</button>
    <button
      class="px-3 py-1.5 text-xs rounded-md transition-colors
        {mode === 'yaml' ? 'bg-surface-3 text-text-primary' : 'text-text-secondary hover:text-text-primary'}"
      onclick={() => switchMode('yaml')}
    >YAML</button>
  </div>

  {#if mode === 'visual'}
    <VisualEditor {compose} onchange={(updated) => { compose = updated }} onerrors={(errs) => { hasValidationErrors = Object.keys(errs).length > 0 }} />
  {:else}
    <YamlEditor value={currentYaml} error={yamlError} onchange={(val) => {
      currentYaml = val
      try { yaml.load(val); yamlError = '' } catch (e) { yamlError = e.message }
    }} />
  {/if}

  <div class="sticky bottom-0 bg-surface-0 border-t border-border py-3 mt-4 flex justify-end">
    <Button onclick={handleSave} loading={saving} disabled={mode === 'visual' && hasValidationErrors}>Save &amp; Deploy</Button>
  </div>

  {#if versions.length > 0}
    <div class="bg-surface-2 border border-border rounded-lg p-4 mt-4">
      <h3 class="text-sm font-semibold text-text-primary mb-3">Deploy History</h3>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead><tr class="border-b border-border">
            <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Version</th>
            <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Hash</th>
            <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Deployed</th>
            <th class="py-2 px-3"></th>
          </tr></thead>
          <tbody class="divide-y divide-border-muted">
            {#each versions as v}
              <tr class="hover:bg-surface-1">
                <td class="py-2 px-3">v{v.version}</td>
                <td class="py-2 px-3 font-mono text-xs">{v.hash?.slice(0, 12)}</td>
                <td class="py-2 px-3">{v.created_at ? new Date(v.created_at).toLocaleString() : '-'}</td>
                <td class="py-2 px-3">
                  <Button variant="secondary" size="sm" onclick={() => rollbackTarget = v.id}>Rollback</Button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}

  {#if events.length > 0}
    <div class="bg-surface-2 border border-border rounded-lg p-4 mt-4">
      <h3 class="text-sm font-semibold text-text-primary mb-3">Deploy Events</h3>
      <div class="space-y-2">
        {#each events as evt}
          <div class="flex items-center gap-3 text-sm px-2 py-1.5 bg-surface-1 rounded">
            <Badge variant={evt.action === 'deploy' ? 'success' : evt.action === 'rollback' ? 'warning' : 'info'}>{evt.action}</Badge>
            <span class="text-text-secondary flex-1">{evt.detail || '-'}</span>
            <span class="text-xs text-text-muted">{evt.created_at ? new Date(evt.created_at).toLocaleString() : ''}</span>
          </div>
        {/each}
      </div>
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
