<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let registries = $state([])
  let loading = $state(true)
  let showPanel = $state(false)

  let rName = $state('')
  let rURL = $state('')
  let rUsername = $state('')
  let rPassword = $state('')

  onMount(loadRegistries)

  async function loadRegistries() {
    loading = true
    const res = await api.listRegistries()
    registries = res.data || []
    loading = false
  }

  async function addRegistry() {
    const res = await api.createRegistry({ name: rName, url: rURL, username: rUsername, password: rPassword })
    if (!res.error) {
      rName = ''; rURL = ''; rUsername = ''; rPassword = ''
      showPanel = false
      loadRegistries()
    }
  }

  async function deleteRegistry(id) {
    await api.deleteRegistry(id)
    loadRegistries()
  }
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">Registries</h1>
    <Button size="sm" variant="secondary" onclick={() => showPanel = true}>Add Registry</Button>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>
  {:else}
    <div class="bg-surface-2 border border-border rounded-lg p-4">
      {#if registries.length === 0}
        <p class="text-sm text-text-secondary">No registries configured.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Name</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">URL</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Username</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Added</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each registries as r}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3 font-medium">{r.name}</td>
                  <td class="py-2 px-3 text-text-secondary">{r.url}</td>
                  <td class="py-2 px-3 text-text-secondary">{r.username || ''}</td>
                  <td class="py-2 px-3 text-text-secondary">{r.created_at ? new Date(r.created_at).toLocaleDateString() : ''}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => deleteRegistry(r.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <SlidePanel title="Add Registry" open={showPanel} onclose={() => showPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); addRegistry() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Name</label>
        <input bind:value={rName} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">URL</label>
        <input bind:value={rURL} required placeholder="registry.example.com" class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Username</label>
        <input bind:value={rUsername} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Password</label>
        <input type="password" bind:value={rPassword} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <Button type="submit">Add Registry</Button>
    </form>
  </SlidePanel>
</Layout>
