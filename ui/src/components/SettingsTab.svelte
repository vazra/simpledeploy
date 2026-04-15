<script>
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

  // Compose editor mode + ref
  let composeMode = $state('visual')
  let configTabRef = $state(null)

  // Endpoint editing
  let editingEndpointIdx = $state(-1) // -1 = none, >=0 = editing that index, -2 = adding new

  // Advanced
  let showAdvanced = $state(false)
  let editAllowlist = $state(initAllowlist)
  let savingAllowlist = $state(false)

  // Danger Zone
  let showDanger = $state(false)
  let showDeleteModal = $state(false)
  let deleting = $state(false)


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
    editEndpoint = { domain: '', port: '', tls: 'letsencrypt', service: serviceNames[0] || '' }
    editingEndpointIdx = -2
  }

  function cancelEndpointEdit() {
    editingEndpointIdx = -1
  }

  async function saveEndpoint() {
    savingEndpoints = true
    const ep = { ...editEndpoint, port: String(editEndpoint.port || '') }
    let updated = [...endpoints.map(e => ({ ...e, port: String(e.port || '') }))]
    if (editingEndpointIdx === -2) {
      updated.push(ep)
    } else {
      updated[editingEndpointIdx] = ep
    }
    await api.updateEndpoints(slug, updated)
    editingEndpointIdx = -1
    savingEndpoints = false
    onAppUpdated()
    configTabRef?.reload()
  }

  async function deleteEndpoint(i) {
    savingEndpoints = true
    const updated = endpoints.filter((_, idx) => idx !== i).map(e => ({ ...e, port: String(e.port || '') }))
    await api.updateEndpoints(slug, updated)
    savingEndpoints = false
    onAppUpdated()
    configTabRef?.reload()
  }

  async function saveAllowlist() {
    savingAllowlist = true
    await api.updateAccess(slug, editAllowlist)
    savingAllowlist = false
  }

  async function confirmDelete() {
    deleting = true
    await api.removeApp(slug)
    deleting = false
    showDeleteModal = false
    push('/')
  }

  function tlsBadgeVariant(tls) {
    if (tls === 'letsencrypt') return 'success'
    if (tls === 'custom') return 'info'
    if (tls === 'local') return 'warning'
    return 'warning'
  }

  function tlsLabel(tls) {
    if (tls === 'letsencrypt') return 'Auto TLS'
    if (tls === 'custom') return 'Custom TLS'
    if (tls === 'local') return 'Local CA'
    return 'No TLS'
  }

  const inputClass = 'flex-1 bg-surface-3 border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/60 transition-colors'
  const labelClass = 'text-xs font-medium text-text-secondary'
</script>

<div class="space-y-6">

  <!-- Endpoint edit form snippet -->
  {#snippet endpointForm(saveLabel)}
    <div class="bg-surface-1 rounded-lg p-3 border border-accent/30 space-y-2">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div>
          <label class="block text-[11px] text-text-muted mb-0.5">Domain</label>
          <input type="text" bind:value={editEndpoint.domain} placeholder="myapp.example.com" class={inputClass} />
        </div>
        <div>
          <label class="block text-[11px] text-text-muted mb-0.5">Service</label>
          <select bind:value={editEndpoint.service} class={inputClass}>
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
            <option value="local">Local (internal CA)</option>
            <option value="off">Off</option>
          </select>
        </div>
      </div>
      {#if editEndpoint.tls === 'local'}
        <div class="col-span-full bg-amber-500/10 border border-amber-500/30 rounded-lg px-3 py-2">
          <p class="text-xs text-amber-400">Browsers will show security warnings unless you install the root certificate on each device.
            <a href="/trust" target="_blank" rel="noopener" class="underline hover:text-amber-300">Install instructions</a>
          </p>
        </div>
      {/if}
      {#if editEndpoint.tls === 'custom' && editEndpoint.domain}
        <div class="bg-surface-2/50 rounded-md p-2.5 space-y-2 border border-border/30">
          <p class="text-[11px] text-text-muted">Custom certificate for <span class="font-mono text-text-primary">{editEndpoint.domain}</span></p>
          <div>
            <label class="block text-[11px] text-text-muted mb-0.5">Certificate (PEM)</label>
            <textarea placeholder="-----BEGIN CERTIFICATE-----" rows="3" id="ep-cert-input"
              class="w-full bg-input-bg border border-border/50 rounded px-2.5 py-1.5 text-xs font-mono text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50 resize-y"></textarea>
          </div>
          <div>
            <label class="block text-[11px] text-text-muted mb-0.5">Private Key (PEM)</label>
            <textarea placeholder="-----BEGIN PRIVATE KEY-----" rows="3" id="ep-key-input"
              class="w-full bg-input-bg border border-border/50 rounded px-2.5 py-1.5 text-xs font-mono text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50 resize-y"></textarea>
          </div>
          <div class="flex justify-end">
            <button type="button"
              onclick={async () => {
                const cert = document.getElementById('ep-cert-input')?.value
                const key = document.getElementById('ep-key-input')?.value
                if (cert && key && editEndpoint.domain) {
                  await api.uploadCert(slug, editEndpoint.domain, cert, key)
                  document.getElementById('ep-cert-input').value = ''
                  document.getElementById('ep-key-input').value = ''
                }
              }}
              class="px-3 py-1.5 text-xs rounded-lg bg-btn-primary hover:bg-btn-primary-hover text-surface-0 transition-colors">
              Upload Certificate
            </button>
          </div>
        </div>
      {/if}
      <div class="flex items-center justify-end gap-2 pt-1">
        <Button variant="ghost" size="sm" onclick={cancelEndpointEdit}>Cancel</Button>
        <Button size="sm" onclick={saveEndpoint} loading={savingEndpoints}>{saveLabel}</Button>
      </div>
    </div>
  {/snippet}

  <!-- Mode toggle -->
  <div class="flex gap-0.5 bg-surface-3/40 rounded-lg p-0.5 w-fit">
    <button
      class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors
        {composeMode === 'visual' ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
      onclick={() => configTabRef?.switchToMode('visual')}
    >Visual</button>
    <button
      class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors
        {composeMode === 'yaml' ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
      onclick={() => configTabRef?.switchToMode('yaml')}
    >YAML</button>
  </div>

  <!-- Section 1: Endpoints (hidden in YAML mode, managed via compose labels there) -->
  {#if composeMode !== 'yaml'}
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
            {@render endpointForm('Save')}
          {:else}
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
    {#if editingEndpointIdx === -2}
      <div class="mt-2">
        {@render endpointForm('Add Endpoint')}
      </div>
    {/if}
  </div>
  {/if}

  <!-- Section 2: Compose Configuration -->
  <ConfigTab bind:this={configTabRef} {slug} composePath={app?.ComposePath} onModeChange={(m) => composeMode = m} />

  <!-- Section 3: Environment Variables (hidden in YAML mode, shown inline there) -->
  {#if composeMode !== 'yaml'}
    <EnvEditor {slug} />
  {/if}

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
              <span class="text-xs font-mono text-text-primary break-all">{app?.ComposePath}</span>
            </div>
            <div class="flex gap-3">
              <span class="text-xs text-text-muted w-24 shrink-0">Env File</span>
              <span class="text-xs font-mono text-text-primary break-all">{app?.ComposePath ? app.ComposeFile.replace(/[^/]+$/, '.env') : '-'}</span>
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

{#if showDeleteModal}
  <Modal
    title="Delete App"
    message="This will permanently remove {app?.Name} and all its data. This cannot be undone."
    onConfirm={confirmDelete}
    onCancel={() => showDeleteModal = false}
  />
{/if}
