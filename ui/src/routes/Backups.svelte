<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Modal from '../components/Modal.svelte'
  import { api } from '../lib/api.js'

  let apps = $state([])
  let selectedApp = $state('')
  let configs = $state([])
  let runs = $state([])
  let error = $state('')
  let loading = $state(true)
  let restoreTarget = $state(null)

  // form
  let strategy = $state('postgres')
  let target = $state('s3')
  let cron = $state('0 2 * * *')
  let retention = $state(7)

  onMount(async () => {
    try { apps = (await api.listApps()) || [] } catch (e) { error = e.message }
    loading = false
  })

  async function loadAppData() {
    if (!selectedApp) { configs = []; runs = []; return }
    try {
      error = ''
      ;[configs, runs] = await Promise.all([
        api.listBackupConfigs(selectedApp).catch(() => []),
        api.listBackupRuns(selectedApp).catch(() => []),
      ])
    } catch (e) { error = e.message }
  }

  async function createConfig() {
    try {
      error = ''
      await api.createBackupConfig(selectedApp, { strategy, target, cron_expr: cron, retention_days: retention })
      await loadAppData()
    } catch (e) { error = e.message }
  }

  async function deleteConfig(id) {
    try { await api.deleteBackupConfig(id); await loadAppData() } catch (e) { error = e.message }
  }

  async function backupNow() {
    try { error = ''; await api.triggerBackup(selectedApp); await loadAppData() } catch (e) { error = e.message }
  }

  async function confirmRestore() {
    if (!restoreTarget) return
    try { error = ''; await api.restore(restoreTarget); restoreTarget = null; await loadAppData() }
    catch (e) { error = e.message; restoreTarget = null }
  }

  function onAppChange(e) { selectedApp = e.target.value; loadAppData() }
</script>

<Layout>
  <h2 class="page-title">Backups</h2>
  {#if error}<div class="error">{error}</div>{/if}

  <div class="section">
    <label class="field-label">Select App</label>
    <select class="input" value={selectedApp} onchange={onAppChange}>
      <option value="">-- choose app --</option>
      {#each apps as app}<option value={app.Slug || app.slug}>{app.Slug || app.slug}</option>{/each}
    </select>
  </div>

  {#if selectedApp}
    <div class="section">
      <h3 class="section-title">New Backup Config</h3>
      <form onsubmit={(e) => { e.preventDefault(); createConfig() }}>
        <label class="field-label">Strategy</label>
        <select class="input" bind:value={strategy}><option>postgres</option><option>volume</option></select>
        <label class="field-label">Target</label>
        <select class="input" bind:value={target}><option>s3</option><option>local</option></select>
        <label class="field-label">Cron Schedule</label>
        <input class="input" bind:value={cron} placeholder="0 2 * * *" />
        <label class="field-label">Retention (days)</label>
        <input class="input" type="number" bind:value={retention} />
        <div class="form-actions"><button class="btn-primary" type="submit">Create Config</button></div>
      </form>
    </div>

    <div class="section">
      <div class="section-header">
        <h3 class="section-title">Backup Configs</h3>
      </div>
      {#if configs.length === 0}<p class="empty">No backup configs.</p>
      {:else}
        <table class="table">
          <thead><tr><th>Strategy</th><th>Target</th><th>Cron</th><th>Retention</th><th></th></tr></thead>
          <tbody>
            {#each configs as c}
              <tr>
                <td>{c.strategy}</td><td>{c.target}</td><td>{c.cron_expr}</td><td>{c.retention_days}d</td>
                <td><button class="btn-danger-sm" onclick={() => deleteConfig(c.id)}>Delete</button></td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>

    <div class="section">
      <div class="section-header">
        <h3 class="section-title">Backup Runs</h3>
        <button class="btn-primary" onclick={backupNow}>Backup Now</button>
      </div>
      {#if runs.length === 0}<p class="empty">No backup runs.</p>
      {:else}
        <table class="table">
          <thead><tr><th>ID</th><th>Status</th><th>Started</th><th>Finished</th><th></th></tr></thead>
          <tbody>
            {#each runs as r}
              <tr>
                <td>{r.id}</td>
                <td><span class="badge" class:success={r.status==='completed'} class:fail={r.status==='failed'}>{r.status}</span></td>
                <td>{r.started_at ? new Date(r.started_at).toLocaleString() : '-'}</td>
                <td>{r.finished_at ? new Date(r.finished_at).toLocaleString() : '-'}</td>
                <td><button class="btn-sm" onclick={() => restoreTarget = r.id}>Restore</button></td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {/if}

  {#if restoreTarget}
    <Modal title="Confirm Restore" message="This will restore the backup. Are you sure?" onConfirm={confirmRestore} onCancel={() => restoreTarget = null} />
  {/if}
</Layout>

<style>
  .page-title { font-size: 1.1rem; font-weight: 600; color: #e1e4e8; margin: 0 0 1rem; }
  .section { background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px; padding: 1rem; margin-bottom: 1rem; }
  .section-title { font-size: 0.9rem; font-weight: 600; color: #e1e4e8; margin: 0 0 0.75rem; }
  .section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; }
  .section-header .section-title { margin: 0; }
  .field-label { display: block; font-size: 0.75rem; color: #8b949e; margin: 0.5rem 0 0.2rem; }
  .input { width: 100%; padding: 0.4rem 0.5rem; background: #0d1117; border: 1px solid #30363d; border-radius: 4px; color: #e1e4e8; font-size: 0.85rem; box-sizing: border-box; }
  .form-actions { display: flex; justify-content: flex-end; margin-top: 0.75rem; }
  .btn-primary { padding: 0.4rem 0.8rem; background: #238636; border: none; border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.8rem; }
  .btn-primary:hover { background: #2ea043; }
  .btn-sm { padding: 0.25rem 0.5rem; background: #30363d; border: none; border-radius: 4px; color: #e1e4e8; cursor: pointer; font-size: 0.75rem; }
  .btn-danger-sm { padding: 0.25rem 0.5rem; background: #da3633; border: none; border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.75rem; }
  .error { color: #f85149; font-size: 0.85rem; margin-bottom: 0.75rem; }
  .empty { color: #8b949e; font-size: 0.85rem; }
  .table { width: 100%; border-collapse: collapse; font-size: 0.82rem; }
  .table th { text-align: left; color: #8b949e; font-weight: 500; padding: 0.4rem 0.5rem; }
  .table td { padding: 0.4rem 0.5rem; color: #e1e4e8; }
  .table tbody tr:nth-child(even) { background: #161b22; }
  .badge { padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.72rem; }
  .badge.success { background: #238636; color: #fff; }
  .badge.fail { background: #da3633; color: #fff; }
</style>
