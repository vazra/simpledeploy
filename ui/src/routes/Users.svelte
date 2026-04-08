<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import { api } from '../lib/api.js'

  let users = $state([])
  let keys = $state([])
  let error = $state('')
  let newKey = $state('')

  // user form
  let uName = $state('')
  let uPass = $state('')
  let uRole = $state('viewer')

  // key form
  let kName = $state('')

  onMount(loadAll)

  async function loadAll() {
    try {
      error = ''
      ;[users, keys] = await Promise.all([
        api.listUsers().catch(() => []),
        api.listAPIKeys().catch(() => []),
      ])
    } catch (e) { error = e.message }
  }

  async function createUser() {
    try { error = ''; await api.createUser({ username: uName, password: uPass, role: uRole }); uName = ''; uPass = ''; await loadAll() }
    catch (e) { error = e.message }
  }

  async function delUser(id) {
    try { await api.deleteUser(id); await loadAll() } catch (e) { error = e.message }
  }

  async function createKey() {
    try {
      error = ''; newKey = ''
      const res = await api.createAPIKey(kName)
      newKey = res.key
      kName = ''
      await loadAll()
    } catch (e) { error = e.message }
  }

  async function revokeKey(id) {
    try { await api.deleteAPIKey(id); await loadAll() } catch (e) { error = e.message }
  }

  function copyKey() {
    navigator.clipboard.writeText(newKey)
  }
</script>

<Layout>
  <h2 class="page-title">Users & API Keys</h2>
  {#if error}<div class="error">{error}</div>{/if}

  <!-- Users -->
  <div class="section">
    <h3 class="section-title">Users</h3>
    {#if users.length === 0}<p class="empty">No users.</p>
    {:else}
      <table class="table">
        <thead><tr><th>ID</th><th>Username</th><th>Role</th><th></th></tr></thead>
        <tbody>
          {#each users as u}
            <tr><td>{u.id}</td><td>{u.username}</td><td>{u.role}</td>
              <td><button class="btn-danger-sm" onclick={() => delUser(u.id)}>Delete</button></td></tr>
          {/each}
        </tbody>
      </table>
    {/if}
    <form class="inline-form" onsubmit={(e) => { e.preventDefault(); createUser() }}>
      <input class="input" placeholder="Username" bind:value={uName} required />
      <input class="input" type="password" placeholder="Password" bind:value={uPass} required />
      <select class="input sm" bind:value={uRole}><option>super_admin</option><option>admin</option><option>viewer</option></select>
      <button class="btn-primary" type="submit">Add User</button>
    </form>
  </div>

  <!-- API Keys -->
  <div class="section">
    <h3 class="section-title">API Keys</h3>
    {#if newKey}
      <div class="key-display">
        <span class="key-label">New key (copy now, shown once):</span>
        <div class="key-row">
          <code class="key-value">{newKey}</code>
          <button class="btn-sm" onclick={copyKey}>Copy</button>
        </div>
      </div>
    {/if}
    {#if keys.length === 0}<p class="empty">No API keys.</p>
    {:else}
      <table class="table">
        <thead><tr><th>Name</th><th>Created</th><th></th></tr></thead>
        <tbody>
          {#each keys as k}
            <tr><td>{k.name}</td><td>{new Date(k.created_at).toLocaleString()}</td>
              <td><button class="btn-danger-sm" onclick={() => revokeKey(k.id)}>Revoke</button></td></tr>
          {/each}
        </tbody>
      </table>
    {/if}
    <form class="inline-form" onsubmit={(e) => { e.preventDefault(); createKey() }}>
      <input class="input" placeholder="Key name" bind:value={kName} required />
      <button class="btn-primary" type="submit">Create Key</button>
    </form>
  </div>
</Layout>

<style>
  .page-title { font-size: 1.1rem; font-weight: 600; color: #e1e4e8; margin: 0 0 1rem; }
  .section { background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px; padding: 1rem; margin-bottom: 1rem; }
  .section-title { font-size: 0.9rem; font-weight: 600; color: #e1e4e8; margin: 0 0 0.75rem; }
  .error { color: #f85149; font-size: 0.85rem; margin-bottom: 0.75rem; }
  .empty { color: #8b949e; font-size: 0.85rem; margin: 0 0 0.75rem; }
  .table { width: 100%; border-collapse: collapse; font-size: 0.82rem; margin-bottom: 0.75rem; }
  .table th { text-align: left; color: #8b949e; font-weight: 500; padding: 0.4rem 0.5rem; }
  .table td { padding: 0.4rem 0.5rem; color: #e1e4e8; }
  .table tbody tr:nth-child(even) { background: #161b22; }
  .inline-form { display: flex; gap: 0.4rem; align-items: center; flex-wrap: wrap; }
  .input { padding: 0.4rem 0.5rem; background: #0d1117; border: 1px solid #30363d; border-radius: 4px; color: #e1e4e8; font-size: 0.8rem; }
  .input.sm { width: 120px; }
  .btn-primary { padding: 0.4rem 0.8rem; background: #238636; border: none; border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.8rem; white-space: nowrap; }
  .btn-primary:hover { background: #2ea043; }
  .btn-danger-sm { padding: 0.25rem 0.5rem; background: #da3633; border: none; border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.75rem; }
  .btn-sm { padding: 0.25rem 0.5rem; background: #30363d; border: none; border-radius: 4px; color: #e1e4e8; cursor: pointer; font-size: 0.75rem; }
  .key-display { background: #0d1117; border: 1px solid #3fb950; border-radius: 4px; padding: 0.75rem; margin-bottom: 0.75rem; }
  .key-label { font-size: 0.75rem; color: #3fb950; display: block; margin-bottom: 0.35rem; }
  .key-row { display: flex; align-items: center; gap: 0.5rem; }
  .key-value { flex: 1; font-size: 0.8rem; color: #e1e4e8; word-break: break-all; background: #161b22; padding: 0.35rem 0.5rem; border-radius: 3px; }
</style>
