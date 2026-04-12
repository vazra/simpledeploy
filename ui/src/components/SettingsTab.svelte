<script>
  import { onMount } from 'svelte'
  import { push } from 'svelte-spa-router'
  import { api } from '../lib/api.js'
  import ConfigTab from './ConfigTab.svelte'
  import EnvEditor from './EnvEditor.svelte'
  import Button from './Button.svelte'
  import Badge from './Badge.svelte'
  import Modal from './Modal.svelte'

  let { slug, app, onAppUpdated } = $props()

  // Domain
  let editDomain = $state(app?.Domain || '')
  let savingDomain = $state(false)

  // Advanced
  let showAdvanced = $state(false)
  let editAllowlist = $state(app?.Labels?.['simpledeploy.access.allow'] || '')
  let savingAllowlist = $state(false)

  // Danger Zone
  let showDanger = $state(false)
  let showDeleteModal = $state(false)
  let deleting = $state(false)

  // Backups
  let backupConfigs = $state([])
  let backupRuns = $state([])
  let loadingBackups = $state(true)
  let showNewConfigForm = $state(false)
  let triggeringBackup = $state(false)
  let showRestoreModal = $state(null) // holds run id
  let restoringId = $state(null)

  let newConfig = $state({
    strategy: 'postgres',
    target: 's3',
    cron_expr: '0 2 * * *',
    retention_days: 7,
  })

  onMount(loadBackups)

  async function loadBackups() {
    loadingBackups = true
    const [cfgRes, runsRes] = await Promise.all([
      api.listBackupConfigs(slug),
      api.listBackupRuns(slug),
    ])
    backupConfigs = cfgRes.data || []
    backupRuns = runsRes.data || []
    loadingBackups = false
  }

  async function saveDomain() {
    savingDomain = true
    const res = await api.updateDomain(slug, editDomain)
    savingDomain = false
    if (!res.error) onAppUpdated()
  }

  async function saveAllowlist() {
    savingAllowlist = true
    await api.updateAccess(slug, editAllowlist)
    savingAllowlist = false
  }

  async function triggerBackup() {
    triggeringBackup = true
    await api.triggerBackup(slug)
    triggeringBackup = false
    await loadBackups()
  }

  async function createBackupConfig() {
    const res = await api.createBackupConfig(slug, {
      strategy: newConfig.strategy,
      target: newConfig.target,
      cron_expr: newConfig.cron_expr,
      retention_days: Number(newConfig.retention_days),
    })
    if (!res.error) {
      showNewConfigForm = false
      newConfig = { strategy: 'postgres', target: 's3', cron_expr: '0 2 * * *', retention_days: 7 }
      await loadBackups()
    }
  }

  async function deleteBackupConfig(id) {
    await api.deleteBackupConfig(id)
    await loadBackups()
  }

  async function confirmRestore() {
    if (!showRestoreModal) return
    restoringId = showRestoreModal
    await api.restore(showRestoreModal)
    restoringId = null
    showRestoreModal = null
  }

  async function confirmDelete() {
    deleting = true
    await api.removeApp(slug)
    deleting = false
    showDeleteModal = false
    push('/')
  }

  function runStatusVariant(status) {
    if (status === 'success') return 'success'
    if (status === 'failed') return 'danger'
    if (status === 'running') return 'info'
    return 'default'
  }

  const inputClass = 'flex-1 bg-surface-3 border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/60 transition-colors'
  const labelClass = 'text-xs font-medium text-text-secondary'
</script>

<div class="space-y-6">

  <!-- Section 1: Compose Configuration -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <h3 class="text-sm font-medium text-text-primary mb-4">Compose Configuration</h3>
    <ConfigTab {slug} />
  </div>

  <!-- Section 2: Environment Variables -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <h3 class="text-sm font-medium text-text-primary mb-4">Environment Variables</h3>
    <EnvEditor {slug} />
  </div>

  <!-- Section 3: Domain -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <h3 class="text-sm font-medium text-text-primary mb-4">Domain</h3>
    <div class="flex items-center gap-3">
      <label class="{labelClass} whitespace-nowrap" for="domain-input">Domain</label>
      <input
        id="domain-input"
        class={inputClass}
        type="text"
        placeholder="e.g. app.example.com"
        bind:value={editDomain}
      />
      {#if editDomain !== (app?.Domain || '')}
        <Button size="sm" onclick={saveDomain} loading={savingDomain}>Save</Button>
      {/if}
    </div>
  </div>

  <!-- Section 4: Backups -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-sm font-medium text-text-primary">Backups</h3>
      <div class="flex gap-2">
        <Button variant="secondary" size="sm" onclick={triggerBackup} loading={triggeringBackup}>Run Now</Button>
        <Button variant="secondary" size="sm" onclick={() => showNewConfigForm = !showNewConfigForm}>
          {showNewConfigForm ? 'Cancel' : 'New Config'}
        </Button>
      </div>
    </div>

    {#if showNewConfigForm}
      <div class="bg-surface-3/40 rounded-lg p-4 mb-4 space-y-3 border border-border/30">
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="{labelClass} block mb-1" for="new-strategy">Strategy</label>
            <select id="new-strategy" class="{inputClass} w-full" bind:value={newConfig.strategy}>
              <option value="postgres">postgres</option>
              <option value="volume">volume</option>
            </select>
          </div>
          <div>
            <label class="{labelClass} block mb-1" for="new-target">Target</label>
            <select id="new-target" class="{inputClass} w-full" bind:value={newConfig.target}>
              <option value="s3">s3</option>
              <option value="local">local</option>
            </select>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="{labelClass} block mb-1" for="new-cron">Cron Schedule</label>
            <input id="new-cron" class="{inputClass} w-full" type="text" placeholder="0 2 * * *" bind:value={newConfig.cron_expr} />
          </div>
          <div>
            <label class="{labelClass} block mb-1" for="new-retention">Retention (days)</label>
            <input id="new-retention" class="{inputClass} w-full" type="number" min="1" bind:value={newConfig.retention_days} />
          </div>
        </div>
        <div class="flex justify-end">
          <Button size="sm" onclick={createBackupConfig}>Create</Button>
        </div>
      </div>
    {/if}

    {#if loadingBackups}
      <p class="text-xs text-text-muted py-2">Loading...</p>
    {:else}
      {#if backupConfigs.length > 0}
        <div class="overflow-x-auto mb-6">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Strategy</th>
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Target</th>
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Schedule</th>
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Retention</th>
                <th class="py-2 px-3"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/30">
              {#each backupConfigs as cfg}
                <tr class="hover:bg-surface-hover">
                  <td class="py-2 px-3 text-text-primary">{cfg.strategy}</td>
                  <td class="py-2 px-3 text-text-primary">{cfg.target}</td>
                  <td class="py-2 px-3 font-mono text-xs text-text-secondary">{cfg.cron_expr}</td>
                  <td class="py-2 px-3 text-text-secondary">{cfg.retention_days}d</td>
                  <td class="py-2 px-3">
                    <Button variant="ghost" size="sm" onclick={() => deleteBackupConfig(cfg.id)}>Delete</Button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <p class="text-xs text-text-muted mb-4">No backup configs yet.</p>
      {/if}

      {#if backupRuns.length > 0}
        <h4 class="text-xs font-medium text-text-secondary mb-2">Backup Runs</h4>
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">ID</th>
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Status</th>
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Started</th>
                <th class="text-left text-xs font-medium text-text-muted py-2 px-3">Finished</th>
                <th class="py-2 px-3"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/30">
              {#each backupRuns as run}
                <tr class="hover:bg-surface-hover">
                  <td class="py-2 px-3 font-mono text-xs text-text-muted">{run.id}</td>
                  <td class="py-2 px-3">
                    <Badge variant={runStatusVariant(run.status)}>{run.status}</Badge>
                  </td>
                  <td class="py-2 px-3 text-text-secondary text-xs">{run.started_at ? new Date(run.started_at).toLocaleString() : '-'}</td>
                  <td class="py-2 px-3 text-text-secondary text-xs">{run.finished_at ? new Date(run.finished_at).toLocaleString() : '-'}</td>
                  <td class="py-2 px-3">
                    <Button variant="secondary" size="sm" onclick={() => showRestoreModal = run.id}>Restore</Button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}
  </div>

  <!-- Section 5: Advanced (collapsed) -->
  <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden">
    <button
      onclick={() => showAdvanced = !showAdvanced}
      class="flex items-center justify-between w-full px-5 py-4 text-left hover:bg-surface-hover transition-colors"
    >
      <h3 class="text-sm font-medium text-text-primary">Advanced</h3>
      <svg
        class="w-4 h-4 text-text-muted transition-transform {showAdvanced ? 'rotate-180' : ''}"
        fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    {#if showAdvanced}
      <div class="px-5 pb-5 space-y-5 border-t border-border/30">
        <!-- IP Allowlist -->
        <div class="pt-4">
          <label class="{labelClass} block mb-1" for="allowlist-input">IP Allowlist</label>
          <div class="flex items-center gap-3">
            <input
              id="allowlist-input"
              class={inputClass}
              type="text"
              placeholder="e.g. 1.2.3.4, 10.0.0.0/8"
              bind:value={editAllowlist}
            />
            {#if editAllowlist !== (app?.Labels?.['simpledeploy.access.allow'] || '')}
              <Button size="sm" onclick={saveAllowlist} loading={savingAllowlist}>Save</Button>
            {/if}
          </div>
          <p class="text-xs text-text-muted mt-1.5">
            {editAllowlist ? 'Only these IPs/CIDRs can access this app' : 'All traffic allowed'}
          </p>
        </div>

        <!-- App Info -->
        <div>
          <p class="text-xs font-medium text-text-secondary mb-2">App Info</p>
          <div class="space-y-1.5 text-sm">
            <div class="flex gap-3">
              <span class="text-xs text-text-muted w-24 shrink-0">Slug</span>
              <span class="text-xs text-text-primary">{app?.Slug}</span>
            </div>
            <div class="flex gap-3">
              <span class="text-xs text-text-muted w-24 shrink-0">Compose File</span>
              <span class="text-xs font-mono text-text-primary break-all">{app?.ComposeFile}</span>
            </div>
            <div class="flex gap-3">
              <span class="text-xs text-text-muted w-24 shrink-0">Created</span>
              <span class="text-xs text-text-primary">{app?.CreatedAt ? new Date(app.CreatedAt).toLocaleDateString() : '-'}</span>
            </div>
          </div>
        </div>
      </div>
    {/if}
  </div>

  <!-- Section 6: Danger Zone (collapsed) -->
  <div class="bg-surface-2 rounded-xl shadow-sm border {showDanger ? 'border-red-500/30' : 'border-border/50'} overflow-hidden transition-colors">
    <button
      onclick={() => showDanger = !showDanger}
      class="flex items-center justify-between w-full px-5 py-4 text-left hover:bg-surface-hover transition-colors"
    >
      <h3 class="text-sm font-medium {showDanger ? 'text-red-400' : 'text-text-primary'}">Danger Zone</h3>
      <svg
        class="w-4 h-4 text-text-muted transition-transform {showDanger ? 'rotate-180' : ''}"
        fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    {#if showDanger}
      <div class="px-5 pb-5 border-t border-red-500/20">
        <div class="pt-4 flex items-center justify-between gap-4">
          <p class="text-sm text-text-secondary">
            This will permanently remove <span class="font-medium text-text-primary">{app?.Name}</span> and all its data.
          </p>
          <Button variant="danger" size="sm" onclick={() => showDeleteModal = true} loading={deleting}>
            Delete App
          </Button>
        </div>
      </div>
    {/if}
  </div>

</div>

{#if showRestoreModal}
  <Modal
    title="Restore Backup"
    message="This will restore the app from this backup. The current state will be overwritten. Continue?"
    onConfirm={confirmRestore}
    onCancel={() => showRestoreModal = null}
  />
{/if}

{#if showDeleteModal}
  <Modal
    title="Delete App"
    message="This will permanently remove {app?.Name} and all its data. This cannot be undone."
    onConfirm={confirmDelete}
    onCancel={() => showDeleteModal = false}
  />
{/if}
