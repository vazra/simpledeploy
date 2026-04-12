<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import FormModal from '../components/FormModal.svelte'
  import Modal from '../components/Modal.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let users = $state([])
  let keys = $state([])
  let newKey = $state('')
  let newKeyName = $state('')
  let loading = $state(true)

  let showUserModal = $state(false)
  let showKeyModal = $state(false)

  // user form
  let uName = $state('')
  let uPass = $state('')
  let uRole = $state('viewer')

  // key form
  let kName = $state('')

  // delete confirmation
  let confirmModal = $state({ open: false, title: '', message: '' , onConfirm: () => {} })

  const roleVariants = {
    super_admin: 'danger',
    admin: 'warning',
    viewer: 'info',
  }

  const roleCircleColors = {
    super_admin: 'bg-red-500/10 text-red-400 light:bg-red-50 light:text-red-700',
    admin: 'bg-amber-500/10 text-amber-400 light:bg-amber-50 light:text-amber-700',
    viewer: 'bg-blue-500/10 text-blue-400 light:bg-blue-50 light:text-blue-700',
  }

  function getInitials(name) {
    return name.slice(0, 2).toUpperCase()
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

  function confirmDelete(title, message, action) {
    confirmModal = { open: true, title, message, onConfirm: action }
  }

  function closeConfirm() {
    confirmModal = { open: false, title: '', message: '', onConfirm: () => {} }
  }

  async function createUser() {
    const res = await api.createUser({ username: uName, password: uPass, role: uRole })
    if (!res.error) { uName = ''; uPass = ''; uRole = 'viewer'; showUserModal = false; loadAll() }
  }

  async function delUser(id) { await api.deleteUser(id); loadAll() }

  async function createKey() {
    newKey = ''
    const res = await api.createAPIKey(kName)
    if (!res.error) {
      newKey = res.data?.key || ''
      newKeyName = kName
      kName = ''
      showKeyModal = false
      loadAll()
    }
  }

  async function revokeKey(id) { await api.deleteAPIKey(id); loadAll() }

  function copyKey() {
    navigator.clipboard.writeText(newKey)
  }
</script>

<Layout>
  <div class="flex flex-wrap items-center justify-between gap-3 mb-5">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Users & API Keys</h1>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>
  {:else}
    <!-- Users -->
    <div class="bg-surface-2 rounded-xl p-6 shadow-sm border border-border/50 mb-6">
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2.5">
          <h3 class="text-sm font-semibold text-text-primary">Users</h3>
          {#if users.length > 0}
            <Badge>{users.length}</Badge>
          {/if}
        </div>
        <Button size="sm" variant="secondary" onclick={() => showUserModal = true}>Add User</Button>
      </div>
      {#if users.length === 0}
        <div class="flex flex-col items-center justify-center py-10 text-center">
          <svg class="w-12 h-12 text-text-muted/40 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0ZM4 19.235v-.11a6.375 6.375 0 0 1 12.75 0v.109A12.318 12.318 0 0 1 10.374 21c-2.331 0-4.512-.645-6.374-1.766Z" />
          </svg>
          <p class="text-sm text-text-muted mb-3">No users yet</p>
          <Button size="sm" variant="secondary" onclick={() => showUserModal = true}>Add User</Button>
        </div>
      {:else}
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {#each users as u}
            <div class="bg-surface-1 border border-border/50 rounded-xl p-4">
              <div class="flex items-center gap-3 mb-3">
                <div class="w-10 h-10 rounded-full flex items-center justify-center text-xs font-semibold shrink-0 {roleCircleColors[u.role] || 'bg-surface-3/60 text-text-secondary'}">
                  {getInitials(u.username)}
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="font-medium text-sm text-text-primary truncate">{u.username}</span>
                    <Badge variant={roleVariants[u.role] || 'default'}>{u.role}</Badge>
                  </div>
                  <span class="text-xs text-text-muted">Created {u.created_at ? new Date(u.created_at).toLocaleDateString() : 'N/A'}</span>
                </div>
              </div>
              <div class="flex items-center justify-end">
                <Button variant="danger" size="sm" onclick={() => confirmDelete('Delete User?', `This will permanently remove "${u.username}".`, () => delUser(u.id))}>Delete</Button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- API Keys -->
    <div class="bg-surface-2 rounded-xl p-6 shadow-sm border border-border/50">
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2.5">
          <h3 class="text-sm font-semibold text-text-primary">API Keys</h3>
          {#if keys.length > 0}
            <Badge>{keys.length}</Badge>
          {/if}
        </div>
        <Button size="sm" variant="secondary" onclick={() => showKeyModal = true}>Create Key</Button>
      </div>

      <!-- New Key Display (inside API Keys section) -->
      {#if newKey}
        <div class="bg-emerald-500/10 border border-emerald-500/20 rounded-xl px-5 py-4 mb-4 light:bg-emerald-50 relative">
          <button onclick={() => newKey = ''} class="absolute top-3 right-3 text-emerald-400/60 hover:text-emerald-400 light:text-emerald-600/60 light:hover:text-emerald-700 cursor-pointer">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
          <p class="text-xs text-emerald-400 light:text-emerald-700 mb-2">Key "<strong>{newKeyName}</strong>" created. Copy now, it won't be shown again.</p>
          <div class="flex items-center gap-2">
            <code class="flex-1 text-xs bg-surface-1 text-text-primary px-3 py-2 rounded break-all font-mono">{newKey}</code>
            <Button size="sm" variant="secondary" onclick={copyKey}>Copy</Button>
          </div>
        </div>
      {/if}

      {#if keys.length === 0 && !newKey}
        <div class="flex flex-col items-center justify-center py-10 text-center">
          <svg class="w-12 h-12 text-text-muted/40 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 0 1 3 3m3 0a6 6 0 0 1-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1 1 21.75 8.25Z" />
          </svg>
          <p class="text-sm text-text-muted mb-3">No API keys yet</p>
          <Button size="sm" variant="secondary" onclick={() => showKeyModal = true}>Create Key</Button>
        </div>
      {:else if keys.length > 0}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/30">
              <th class="text-left text-xs font-medium text-text-muted/70 py-3 px-4">Name</th>
              <th class="text-left text-xs font-medium text-text-muted/70 py-3 px-4">Created</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each keys as k}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 font-medium">
                    <div class="flex items-center gap-2">
                      <svg class="w-4 h-4 text-text-muted/60 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 0 1 3 3m3 0a6 6 0 0 1-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1 1 21.75 8.25Z" />
                      </svg>
                      <span>{k.name}</span>
                    </div>
                  </td>
                  <td class="py-3 px-4 text-text-secondary">{k.created_at ? new Date(k.created_at).toLocaleString() : 'Just now'}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => confirmDelete('Revoke API Key?', `This will permanently revoke "${k.name}".`, () => revokeKey(k.id))}>Revoke</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Add User Modal -->
  <FormModal title="Add User" open={showUserModal} onclose={() => showUserModal = false}>
    <form onsubmit={(e) => { e.preventDefault(); createUser() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Username</label>
        <input bind:value={uName} required placeholder="e.g. jane" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Password</label>
        <input type="password" bind:value={uPass} required placeholder="Min 8 characters" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Role</label>
        <div class="grid grid-cols-3 gap-2">
          {#each [
            { value: 'viewer', label: 'Viewer', desc: 'View only' },
            { value: 'admin', label: 'Admin', desc: 'Manage apps' },
            { value: 'super_admin', label: 'Super Admin', desc: 'Full access' },
          ] as role}
            <button
              type="button"
              onclick={() => uRole = role.value}
              class="px-3 py-2.5 rounded-lg border text-left transition-colors cursor-pointer {uRole === role.value ? 'border-accent bg-accent/10 text-text-primary' : 'border-border/50 bg-input-bg text-text-secondary hover:border-text-muted'}"
            >
              <span class="block text-xs font-medium">{role.label}</span>
              <span class="block text-[10px] text-text-muted mt-0.5">{role.desc}</span>
            </button>
          {/each}
        </div>
      </div>
      <Button type="submit">Create User</Button>
    </form>
  </FormModal>

  <!-- Create Key Modal -->
  <FormModal title="Create API Key" open={showKeyModal} onclose={() => showKeyModal = false}>
    <form onsubmit={(e) => { e.preventDefault(); createKey() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Key Name</label>
        <input bind:value={kName} required placeholder="e.g. ci-deploy" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
        <p class="text-[10px] text-text-muted mt-1.5">A label to help you identify this key later.</p>
      </div>
      <Button type="submit">Create Key</Button>
    </form>
  </FormModal>

  <!-- Delete Confirmation Modal -->
  {#if confirmModal.open}
    <Modal title={confirmModal.title} message={confirmModal.message} onConfirm={() => { confirmModal.onConfirm(); closeConfirm() }} onCancel={closeConfirm} />
  {/if}
</Layout>
