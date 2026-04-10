<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import Modal from '../components/Modal.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let apps = $state([])
  let selectedApp = $state('')
  let configs = $state([])
  let runs = $state([])
  let loading = $state(true)
  let restoreTarget = $state(null)
  let showConfigPanel = $state(false)

  // form
  let strategy = $state('postgres')
  let target = $state('s3')
  let cron = $state('0 2 * * *')
  let retention = $state(7)

  onMount(async () => {
    const res = await api.listApps()
    apps = res.data || []
    loading = false
  })

  async function loadAppData() {
    if (!selectedApp) { configs = []; runs = []; return }
    const [cRes, rRes] = await Promise.all([
      api.listBackupConfigs(selectedApp),
      api.listBackupRuns(selectedApp),
    ])
    configs = cRes.data || []
    runs = rRes.data || []
  }

  async function createConfig() {
    const res = await api.createBackupConfig(selectedApp, {
      strategy, target, cron_expr: cron, retention_days: retention,
    })
    if (!res.error) { showConfigPanel = false; loadAppData() }
  }

  async function deleteConfig(id) {
    await api.deleteBackupConfig(id)
    loadAppData()
  }

  async function backupNow() {
    await api.triggerBackup(selectedApp)
    loadAppData()
  }

  async function confirmRestore() {
    if (!restoreTarget) return
    await api.restore(restoreTarget)
    restoreTarget = null
    loadAppData()
  }

  function onAppChange(e) {
    selectedApp = e.target.value
    loadAppData()
  }
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">Backups</h1>
  </div>

  {#if loading}
    <Skeleton type="card" count={2} />
  {:else}
    <!-- App Selector -->
    <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
      <label class="block text-xs text-text-secondary mb-1.5">Select App</label>
      <select
        value={selectedApp}
        onchange={onAppChange}
        class="w-full max-w-xs px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary"
      >
        <option value="">-- choose app --</option>
        {#each apps as app}<option value={app.Slug || app.slug}>{app.Name || app.Slug || app.slug}</option>{/each}
      </select>
    </div>

    {#if selectedApp}
      <!-- Backup Configs -->
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-text-primary">Backup Configs</h3>
          <Button size="sm" variant="secondary" onclick={() => showConfigPanel = true}>New Config</Button>
        </div>
        {#if configs.length === 0}
          <p class="text-sm text-text-secondary">No backup configs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border">
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Strategy</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Target</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Cron</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Retention</th>
                <th class="py-2 px-3"></th>
              </tr></thead>
              <tbody class="divide-y divide-border-muted">
                {#each configs as c}
                  <tr class="hover:bg-surface-1">
                    <td class="py-2 px-3">{c.strategy}</td>
                    <td class="py-2 px-3">{c.target}</td>
                    <td class="py-2 px-3 font-mono text-xs">{c.cron_expr}</td>
                    <td class="py-2 px-3">{c.retention_days}d</td>
                    <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => deleteConfig(c.id)}>Delete</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

      <!-- Backup Runs -->
      <div class="bg-surface-2 border border-border rounded-lg p-4">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-text-primary">Backup Runs</h3>
          <Button size="sm" onclick={backupNow}>Backup Now</Button>
        </div>
        {#if runs.length === 0}
          <p class="text-sm text-text-secondary">No backup runs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border">
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">ID</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Status</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Started</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Finished</th>
                <th class="py-2 px-3"></th>
              </tr></thead>
              <tbody class="divide-y divide-border-muted">
                {#each runs as r}
                  <tr class="hover:bg-surface-1">
                    <td class="py-2 px-3">{r.id}</td>
                    <td class="py-2 px-3"><Badge variant={r.status === 'completed' ? 'success' : 'danger'}>{r.status}</Badge></td>
                    <td class="py-2 px-3">{r.started_at ? new Date(r.started_at).toLocaleString() : '-'}</td>
                    <td class="py-2 px-3">{r.finished_at ? new Date(r.finished_at).toLocaleString() : '-'}</td>
                    <td class="py-2 px-3"><Button variant="secondary" size="sm" onclick={() => restoreTarget = r.id}>Restore</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}
  {/if}

  {#if restoreTarget}
    <Modal title="Confirm Restore" message="This will restore the backup. Are you sure?" onConfirm={confirmRestore} onCancel={() => restoreTarget = null} />
  {/if}

  <!-- New Config Slide Panel -->
  <SlidePanel title="New Backup Config" open={showConfigPanel} onclose={() => showConfigPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createConfig() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Strategy</label>
        <select bind:value={strategy} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>postgres</option><option>volume</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Target</label>
        <select bind:value={target} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>s3</option><option>local</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Cron Schedule</label>
        <input bind:value={cron} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Retention (days)</label>
        <input type="number" bind:value={retention} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary" />
      </div>
      <Button type="submit">Create Config</Button>
    </form>
  </SlidePanel>
</Layout>
