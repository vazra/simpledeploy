<script>
  import { onMount } from 'svelte'
  import { push } from 'svelte-spa-router'
  import { api } from '../lib/api.js'
  import ConfigTab from './ConfigTab.svelte'
  import EnvEditor from './EnvEditor.svelte'
  import Button from './Button.svelte'
  import Badge from './Badge.svelte'
  import Modal from './Modal.svelte'

  let { slug, app, services = [], onAppUpdated } = $props()

  let serviceNames = $derived(services.map(s => s.service || s.name || s).filter(Boolean))

  const initAllowlist = app?.Labels?.['simpledeploy.access.allow'] || ''

  // Section expand states
  let showComposeEditor = $state(false)
  let editingEndpointIdx = $state(-1) // -1 = none, >=0 = editing that index, -2 = adding new

  // Advanced
  let showAdvanced = $state(false)
  let editAllowlist = $state(initAllowlist)
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
  let showRestoreModal = $state(null)
  let restoringId = $state(null)

  let newConfig = $state({
    strategy: 'postgres',
    target: 's3',
    cron_expr: '0 2 * * *',
    retention_days: 7,
  })

  // Endpoints from app response
  let endpoints = $derived(app?.endpoints || [])

  // Endpoint editing state
  let editEndpoint = $state({ domain: '', port: '', tls: 'letsencrypt', service: '' })
  let savingEndpoints = $state(false)

  function startEditEndpoint(i) {
    const ep = endpoints[i]
    editEndpoint = { ...ep }
    editingEndpointIdx = i
  }

  function startAddEndpoint() {
    editEndpoint = { domain: '', port: '', tls: 'letsencrypt', service: '' }
    editingEndpointIdx = -2
  }

  function cancelEndpointEdit() {
    editingEndpointIdx = -1
  }

  async function saveEndpoint() {
    savingEndpoints = true
    let updated = [...endpoints]
    if (editingEndpointIdx === -2) {
      updated.push(editEndpoint)
    } else {
      updated[editingEndpointIdx] = editEndpoint
    }
    await api.updateEndpoints(slug, updated)
    editingEndpointIdx = -1
    savingEndpoints = false
    onAppUpdated()
  }

  async function deleteEndpoint(i) {
    savingEndpoints = true
    const updated = endpoints.filter((_, idx) => idx !== i)
    await api.updateEndpoints(slug, updated)
    savingEndpoints = false
    onAppUpdated()
  }

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

  function tlsBadgeVariant(tls) {
    if (tls === 'letsencrypt') return 'success'
    if (tls === 'custom') return 'info'
    return 'warning'
  }

  function tlsLabel(tls) {
    if (tls === 'letsencrypt') return 'Auto TLS'
    if (tls === 'custom') return 'Custom TLS'
    return 'No TLS'
  }

  const inputClass = 'flex-1 bg-surface-3 border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/60 transition-colors'
  const labelClass = 'text-xs font-medium text-text-secondary'
</script>

<div class="space-y-6">

  <!-- Section 1: Endpoints -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <div class="flex items-center justify-between mb-3">
      <h3 class="text-sm font-medium text-text-primary">Endpoints</h3>
      <Button variant="ghost" size="sm" onclick={startAddEndpoint}>+ Add</Button>
    </div>
    {#if endpoints.length === 0 && editingEndpointIdx !== -2}
      <p class="text-xs text-text-muted">No endpoints configured.</p>
    {:else}
      <div class="space-y-2">
        {#each endpoints as ep, i}
          {#if editingEndpointIdx === i}
            <!-- Inline edit form -->
            <div class="bg-surface-1 rounded-lg p-3 border border-accent/30 space-y-2">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <div>
                  <label class="block text-[11px] text-text-muted mb-0.5">Domain</label>
                  <input type="text" bind:value={editEndpoint.domain} placeholder="myapp.example.com" class={inputClass} />
                </div>
                <div>
                  <label class="block text-[11px] text-text-muted mb-0.5">Service</label>
                  <select bind:value={editEndpoint.service} class={inputClass}>
                    <option value="">Select service</option>
                    {#each serviceNames as svc}
                      <option value={svc}>{svc}</option>
                    {/each}
                  </select>
                </div>
                <div>
                  <label class="block text-[11px] text-text-muted mb-0.5">Port</label>
                  <input type="number" bind:value={editEndpoint.port} placeholder="3000" class={inputClass} />
                </div>
                <div>
                  <label class="block text-[11px] text-text-muted mb-0.5">TLS</label>
                  <select bind:value={editEndpoint.tls} class={inputClass}>
                    <option value="letsencrypt">Let's Encrypt (auto)</option>
                    <option value="custom">Custom certificate</option>
                    <option value="off">Off</option>
                  </select>
                </div>
              </div>
              <div class="flex items-center justify-end gap-2 pt-1">
                <Button variant="ghost" size="sm" onclick={cancelEndpointEdit}>Cancel</Button>
                <Button size="sm" onclick={saveEndpoint} loading={savingEndpoints}>Save</Button>
              </div>
            </div>
          {:else}
            <!-- Read-only card -->
            <div class="flex items-center gap-3 bg-surface-1 rounded-lg px-3 py-2.5 border border-border/30 group">
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-text-primary truncate">{ep.domain || 'No domain'}</div>
                <div class="flex items-center gap-2 mt-0.5">
                  <span class="text-[11px] text-text-muted">{ep.service || '?'}:{ep.port || '?'}</span>
                </div>
              </div>
              <Badge variant={tlsBadgeVariant(ep.tls)}>{tlsLabel(ep.tls)}</Badge>
              <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <button onclick={() => startEditEndpoint(i)} class="p-1 rounded text-text-muted hover:text-accent hover:bg-accent/10 transition-colors" title="Edit">
                  <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" /></svg>
                </button>
                <button onclick={() => deleteEndpoint(i)} class="p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors" title="Delete">
                  <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                </button>
              </div>
            </div>
          {/if}
        {/each}
      </div>
    {/if}
    <!-- Add new endpoint form -->
    {#if editingEndpointIdx === -2}
      <div class="bg-surface-1 rounded-lg p-3 border border-accent/30 space-y-2 mt-2">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <div>
            <label class="block text-[11px] text-text-muted mb-0.5">Domain</label>
            <input type="text" bind:value={editEndpoint.domain} placeholder="myapp.example.com" class={inputClass} />
          </div>
          <div>
            <label class="block text-[11px] text-text-muted mb-0.5">Service</label>
            <select bind:value={editEndpoint.service} class={inputClass}>
              <option value="">Select service</option>
              {#each serviceNames as svc}
                <option value={svc}>{svc}</option>
              {/each}
            </select>
          </div>
          <div>
            <label class="block text-[11px] text-text-muted mb-0.5">Port</label>
            <input type="number" bind:value={editEndpoint.port} placeholder="3000" class={inputClass} />
          </div>
          <div>
            <label class="block text-[11px] text-text-muted mb-0.5">TLS</label>
            <select bind:value={editEndpoint.tls} class={inputClass}>
              <option value="letsencrypt">Let's Encrypt (auto)</option>
              <option value="custom">Custom certificate</option>
              <option value="off">Off</option>
            </select>
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-1">
          <Button variant="ghost" size="sm" onclick={cancelEndpointEdit}>Cancel</Button>
          <Button size="sm" onclick={saveEndpoint} loading={savingEndpoints}>Add Endpoint</Button>
        </div>
      </div>
    {/if}
  </div>

  <!-- Section 2: Compose Configuration -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-sm font-medium text-text-primary">Compose Configuration</h3>
      <Button variant="ghost" size="sm" onclick={() => showComposeEditor = !showComposeEditor}>
        {showComposeEditor ? 'Close Editor' : 'Edit'}
      </Button>
    </div>
    {#if showComposeEditor}
      <ConfigTab {slug} />
    {:else}
      <!-- Read-only summary -->
      {#if Object.keys(app?.Services || compose?.services || {}).length === 0}
        <p class="text-xs text-text-muted">No services configured.</p>
      {:else}
        {@const svcEntries = Object.entries(app?.compose?.services || {})}
        <div class="space-y-2">
          {#each services as svc}
            <div class="flex items-center gap-3 bg-surface-1 rounded-lg px-3 py-2 border border-border/30">
              <span class="text-sm font-mono text-text-primary">{svc.service}</span>
              <Badge variant={svc.state === 'running' ? 'success' : svc.state === 'exited' ? 'danger' : 'warning'}>{svc.state || 'unknown'}</Badge>
              {#if svc.health}
                <Badge variant={svc.health === 'healthy' ? 'success' : 'danger'}>{svc.health}</Badge>
              {/if}
            </div>
          {/each}
          {#if services.length === 0}
            <p class="text-xs text-text-muted">Service details unavailable. Click Edit to view compose file.</p>
          {/if}
        </div>
      {/if}
    {/if}
  </div>

  <!-- Section 3: Environment Variables -->
  <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
    <div class="flex items-center justify-between mb-3">
      <div>
        <h3 class="text-sm font-medium text-text-primary">Environment Variables</h3>
        <p class="text-xs text-text-muted mt-0.5">Stored in <code class="font-mono text-[11px]">.env</code> file</p>
      </div>
    </div>
    <EnvEditor {slug} />
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
              <span class="text-xs text-text-muted">(cannot be changed)</span>
            </div>
            <div class="flex gap-3">
              <span class="text-xs text-text-muted w-24 shrink-0">Compose File</span>
              <span class="text-xs font-mono text-text-primary break-all">{app?.ComposeFile}</span>
            </div>
            <div class="flex gap-3">
              <span class="text-xs text-text-muted w-24 shrink-0">Env File</span>
              <span class="text-xs font-mono text-text-primary break-all">{app?.ComposeFile ? app.ComposeFile.replace(/[^/]+$/, '.env') : '-'}</span>
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
