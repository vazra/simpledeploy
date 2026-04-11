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

  // delete confirmation modal
  let confirmModal = $state({ open: false, title: '', name: '', onConfirm: null })

  function confirmDelete(title, name, action) {
    confirmModal = { open: true, title, name, onConfirm: action }
  }

  function closeModal() {
    confirmModal = { open: false, title: '', name: '', onConfirm: null }
  }

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
  <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Users & API Keys</h1>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>
  {:else}
    <!-- New Key Display -->
    {#if newKey}
      <div class="bg-emerald-500/10 border border-emerald-500/20 rounded-xl px-5 py-4 mb-6 light:bg-emerald-50">
        <p class="text-xs text-emerald-400 light:text-emerald-700 mb-2">New key created (copy now, shown once):</p>
        <div class="flex items-center gap-2">
          <code class="flex-1 text-xs bg-surface-1 text-text-primary px-3 py-2 rounded break-all font-mono">{newKey}</code>
          <Button size="sm" variant="secondary" onclick={copyKey}>Copy</Button>
        </div>
      </div>
    {/if}

    <!-- Users -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Users</h3>
        <Button size="sm" variant="secondary" onclick={() => showUserPanel = true}>Add User</Button>
      </div>
      {#if users.length === 0}
        <p class="text-sm text-text-muted">No users.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">ID</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Username</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Role</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Created</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each users as u}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4">{u.id}</td>
                  <td class="py-3 px-4 font-medium">{u.username}</td>
                  <td class="py-3 px-4"><Badge variant={roleVariants[u.role] || 'default'}>{u.role}</Badge></td>
                  <td class="py-3 px-4 text-text-secondary">{u.created_at ? new Date(u.created_at).toLocaleDateString() : ''}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => confirmDelete('Delete User?', u.username, () => delUser(u.id))}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- API Keys -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">API Keys</h3>
        <Button size="sm" variant="secondary" onclick={() => showKeyPanel = true}>Create Key</Button>
      </div>
      {#if keys.length === 0}
        <p class="text-sm text-text-muted">No API keys.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Name</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Created</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each keys as k}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 font-medium">{k.name}</td>
                  <td class="py-3 px-4 text-text-secondary">{new Date(k.created_at).toLocaleString()}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => confirmDelete('Revoke API Key?', k.name, () => revokeKey(k.id))}>Revoke</Button></td>
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
        <label class="block text-xs text-text-muted mb-2">Username</label>
        <input bind:value={uName} required class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">Password</label>
        <input type="password" bind:value={uPass} required class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">Role</label>
        <select bind:value={uRole} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
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
        <label class="block text-xs text-text-muted mb-2">Key Name</label>
        <input bind:value={kName} required class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <Button type="submit">Create Key</Button>
    </form>
  </SlidePanel>

<!-- Delete Confirmation Modal -->
{#if confirmModal.open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onclick={closeModal} onkeydown={(e) => e.key === 'Escape' && closeModal()}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="bg-surface-2 rounded-xl border border-border/50 shadow-lg max-w-sm w-full mx-4 p-6" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-base font-semibold text-text-primary mb-2">{confirmModal.title}</h3>
      <p class="text-sm text-text-secondary mb-5">This will permanently remove <strong class="text-text-primary">{confirmModal.name}</strong>.</p>
      <div class="flex justify-end gap-3">
        <Button size="sm" variant="secondary" onclick={closeModal}>Cancel</Button>
        <Button size="sm" variant="danger" onclick={() => { confirmModal.onConfirm(); closeModal() }}>Confirm</Button>
      </div>
    </div>
  </div>
{/if}
</Layout>
