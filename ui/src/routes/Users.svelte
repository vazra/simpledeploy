<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let users = $state([])
  let keys = $state([])
  let newKey = $state('')
  let loading = $state(true)

  let showUserPanel = $state(false)
  let showKeyPanel = $state(false)

  // user form
  let uName = $state('')
  let uPass = $state('')
  let uRole = $state('viewer')

  // key form
  let kName = $state('')

  const roleVariants = {
    super_admin: 'danger',
    admin: 'warning',
    viewer: 'info',
  }

  onMount(loadAll)

  async function loadAll() {
    loading = true
    const [uRes, kRes] = await Promise.all([
      api.listUsers(),
      api.listAPIKeys(),
    ])
    users = uRes.data || []
    keys = kRes.data || []
    loading = false
  }

  async function createUser() {
    const res = await api.createUser({ username: uName, password: uPass, role: uRole })
    if (!res.error) { uName = ''; uPass = ''; showUserPanel = false; loadAll() }
  }

  async function delUser(id) { await api.deleteUser(id); loadAll() }

  async function createKey() {
    newKey = ''
    const res = await api.createAPIKey(kName)
    if (!res.error) {
      newKey = res.data?.key || ''
      kName = ''
      showKeyPanel = false
      loadAll()
    }
  }

  async function revokeKey(id) { await api.deleteAPIKey(id); loadAll() }

  function copyKey() {
    navigator.clipboard.writeText(newKey)
  }
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">Users & API Keys</h1>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>
  {:else}
    <!-- New Key Display -->
    {#if newKey}
      <div class="bg-green-900/20 border border-success rounded-lg px-4 py-3 mb-4 light:bg-green-50">
        <p class="text-xs text-success mb-2">New key created (copy now, shown once):</p>
        <div class="flex items-center gap-2">
          <code class="flex-1 text-xs bg-surface-1 text-text-primary px-3 py-2 rounded break-all font-mono">{newKey}</code>
          <Button size="sm" variant="secondary" onclick={copyKey}>Copy</Button>
        </div>
      </div>
    {/if}

    <!-- Users -->
    <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Users</h3>
        <Button size="sm" variant="secondary" onclick={() => showUserPanel = true}>Add User</Button>
      </div>
      {#if users.length === 0}
        <p class="text-sm text-text-secondary">No users.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">ID</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Username</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Role</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Created</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each users as u}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3">{u.id}</td>
                  <td class="py-2 px-3 font-medium">{u.username}</td>
                  <td class="py-2 px-3"><Badge variant={roleVariants[u.role] || 'default'}>{u.role}</Badge></td>
                  <td class="py-2 px-3 text-text-secondary">{u.created_at ? new Date(u.created_at).toLocaleDateString() : ''}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => delUser(u.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- API Keys -->
    <div class="bg-surface-2 border border-border rounded-lg p-4">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">API Keys</h3>
        <Button size="sm" variant="secondary" onclick={() => showKeyPanel = true}>Create Key</Button>
      </div>
      {#if keys.length === 0}
        <p class="text-sm text-text-secondary">No API keys.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Name</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Created</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each keys as k}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3 font-medium">{k.name}</td>
                  <td class="py-2 px-3 text-text-secondary">{new Date(k.created_at).toLocaleString()}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => revokeKey(k.id)}>Revoke</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Add User Slide Panel -->
  <SlidePanel title="Add User" open={showUserPanel} onclose={() => showUserPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createUser() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Username</label>
        <input bind:value={uName} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Password</label>
        <input type="password" bind:value={uPass} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Role</label>
        <select bind:value={uRole} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>viewer</option><option>admin</option><option>super_admin</option>
        </select>
      </div>
      <Button type="submit">Create User</Button>
    </form>
  </SlidePanel>

  <!-- Create Key Slide Panel -->
  <SlidePanel title="Create API Key" open={showKeyPanel} onclose={() => showKeyPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createKey() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Key Name</label>
        <input bind:value={kName} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <Button type="submit">Create Key</Button>
    </form>
  </SlidePanel>
</Layout>
