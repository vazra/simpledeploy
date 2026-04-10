<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Modal from '../components/Modal.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'
  import { toasts } from '../lib/stores/toast.js'

  let activeTab = $state('cleanup')
  let loading = $state(false)

  // Docker info state
  let dockerInfo = $state(null)

  // Disk Cleanup state
  let diskUsage = $state(null)
  let pruning = $state(false)
  let pruneModal = $state(null) // { title, message, action }

  // Images state
  let images = $state([])
  let imageToDelete = $state(null)

  // Networks & Volumes state
  let networks = $state([])
  let volumes = $state([])
  let networkToDelete = $state(null)
  let volumeToDelete = $state(null)
  let pruneVolumesConfirm = $state(false)

  function formatBytes(bytes) {
    if (bytes == null || bytes === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i]
  }

  function formatDate(ts) {
    if (!ts) return ''
    const d = typeof ts === 'number' ? new Date(ts * 1000) : new Date(ts)
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  onMount(() => {
    loadDockerInfo()
    loadCleanup()
  })

  async function loadDockerInfo() {
    const res = await api.dockerInfo()
    if (res.data) dockerInfo = res.data
  }

  function switchTab(tab) {
    activeTab = tab
    if (tab === 'cleanup') loadCleanup()
    if (tab === 'images') loadImages()
    if (tab === 'netsvols') loadNetsVols()
  }

  async function loadCleanup() {
    loading = true
    const res = await api.dockerDiskUsage()
    if (res.data) diskUsage = res.data
    loading = false
  }

  async function loadImages() {
    loading = true
    const res = await api.dockerImages()
    images = res.data || []
    loading = false
  }

  async function loadNetsVols() {
    loading = true
    const [nRes, vRes] = await Promise.all([api.dockerNetworks(), api.dockerVolumes()])
    networks = nRes.data || []
    volumes = vRes.data?.Volumes || vRes.data || []
    loading = false
  }

  function sumSize(items, field = 'Size') {
    if (!items) return 0
    return items.reduce((acc, i) => acc + (i[field] || 0), 0)
  }

  function diskCards() {
    if (!diskUsage) return []
    return [
      { label: 'Images', count: diskUsage.Images?.length || 0, size: sumSize(diskUsage.Images), action: () => confirmPrune('Prune Images', 'Remove all dangling images?', doPruneImages) },
      { label: 'Containers', count: diskUsage.Containers?.length || 0, size: sumSize(diskUsage.Containers, 'SizeRw'), action: () => confirmPrune('Prune Containers', 'Remove all stopped containers?', doPruneContainers) },
      { label: 'Volumes', count: diskUsage.Volumes?.length || 0, size: sumSize(diskUsage.Volumes), action: () => confirmPrune('Prune Volumes', 'Remove all unused volumes? This cannot be undone.', doPruneVolumes) },
      { label: 'Build Cache', count: diskUsage.BuildCache?.length || 0, size: sumSize(diskUsage.BuildCache), action: () => confirmPrune('Prune Build Cache', 'Remove all build cache?', doPruneBuildCache) },
    ]
  }

  function confirmPrune(title, message, action) {
    pruneModal = { title, message, action }
  }

  function pruneToast(res, countField, label) {
    if (res.error) { toasts.error(res.error); return }
    const d = res.data || {}
    const count = d[countField]?.length || 0
    const space = formatBytes(d.SpaceReclaimed || 0)
    toasts.success(`${label}: ${count} removed, ${space} reclaimed`)
  }

  async function runPrune(fn) {
    pruning = true
    pruneModal = null
    try { await fn() } finally { pruning = false }
  }

  async function doPruneImages() { await runPrune(async () => {
    const res = await api.dockerPruneImages()
    pruneToast(res, 'ImagesDeleted', 'Images')
    await loadCleanup()
  })}
  async function doPruneContainers() { await runPrune(async () => {
    const res = await api.dockerPruneContainers()
    pruneToast(res, 'ContainersDeleted', 'Containers')
    await loadCleanup()
  })}
  async function doPruneVolumes() { await runPrune(async () => {
    const res = await api.dockerPruneVolumes()
    pruneToast(res, 'VolumesDeleted', 'Volumes')
    await loadCleanup()
  })}
  async function doPruneBuildCache() { await runPrune(async () => {
    const res = await api.dockerPruneBuildCache()
    pruneToast(res, 'CachesDeleted', 'Build cache')
    await loadCleanup()
  })}
  async function doPruneAll() { await runPrune(async () => {
    const res = await api.dockerPruneAll()
    if (res.error) { toasts.error(res.error); return }
    const d = res.data || {}
    const space = formatBytes(d.space_reclaimed || 0)
    toasts.success(`System pruned: ${space} reclaimed`)
    await loadCleanup()
  })}

  function parseRepoTag(img) {
    const tag = img.RepoTags?.[0] || '<none>:<none>'
    const idx = tag.lastIndexOf(':')
    if (idx === -1) return { repo: tag, tag: '' }
    return { repo: tag.slice(0, idx), tag: tag.slice(idx + 1) }
  }

  function shortId(id) {
    return (id || '').replace('sha256:', '').slice(0, 12)
  }

  async function deleteImage() {
    if (!imageToDelete) return
    await api.dockerRemoveImage(imageToDelete)
    imageToDelete = null
    loadImages()
  }

  async function deleteNetwork() {
    if (!networkToDelete) return
    await api.dockerRemoveNetwork(networkToDelete)
    networkToDelete = null
    loadNetsVols()
  }

  async function deleteVolume() {
    if (!volumeToDelete) return
    await api.dockerRemoveVolume(volumeToDelete)
    volumeToDelete = null
    loadNetsVols()
  }

  async function pruneUnusedVolumes() {
    pruneVolumesConfirm = false
    await runPrune(async () => {
      const res = await api.dockerPruneVolumes()
      pruneToast(res, 'VolumesDeleted', 'Volumes')
      await loadNetsVols()
    })
  }

  const systemNetworks = ['bridge', 'host', 'none']
</script>

<Layout>
  <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Docker</h1>
  </div>

  {#if dockerInfo}
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-8">
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
        <div>
          <div class="text-xs font-medium text-text-secondary">Version</div>
          <div class="text-sm font-semibold text-text-primary">{dockerInfo.server_version}</div>
        </div>
        <div>
          <div class="text-xs font-medium text-text-secondary">OS / Arch</div>
          <div class="text-sm font-semibold text-text-primary">{dockerInfo.os} ({dockerInfo.arch})</div>
        </div>
        <div>
          <div class="text-xs font-medium text-text-secondary">CPUs / Memory</div>
          <div class="text-sm font-semibold text-text-primary">{dockerInfo.cpus} cores / {formatBytes(dockerInfo.memory)}</div>
        </div>
        <div>
          <div class="text-xs font-medium text-text-secondary">Containers</div>
          <div class="text-sm font-semibold text-text-primary">
            <span class="text-green-500">{dockerInfo.containers_running}</span> /
            <span class="text-yellow-500">{dockerInfo.containers_paused}</span> /
            <span class="text-text-secondary">{dockerInfo.containers_stopped}</span>
          </div>
          <div class="text-xs text-text-secondary">run / pause / stop</div>
        </div>
        <div>
          <div class="text-xs font-medium text-text-secondary">Images</div>
          <div class="text-sm font-semibold text-text-primary">{dockerInfo.images}</div>
        </div>
        <div>
          <div class="text-xs font-medium text-text-secondary">Storage Driver</div>
          <div class="text-sm font-semibold text-text-primary">{dockerInfo.storage_driver}</div>
        </div>
      </div>
    </div>
  {/if}

  <div class="flex overflow-x-auto gap-1 mb-6 border-b border-border/50">
    {#each [['cleanup', 'Disk Cleanup'], ['images', 'Images'], ['netsvols', 'Networks & Volumes']] as [key, label]}
      <button
        onclick={() => switchTab(key)}
        class="px-4 py-3 text-sm font-medium border-b-2 whitespace-nowrap shrink-0 transition-colors {activeTab === key ? 'border-accent text-accent' : 'border-transparent text-text-muted hover:text-text-primary'}"
      >{label}</button>
    {/each}
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={3} />
    </div>
  {:else if activeTab === 'cleanup'}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      {#each diskCards() as card}
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
          <div class="text-xs font-medium text-text-secondary mb-1">{card.label}</div>
          <div class="text-2xl font-semibold text-text-primary">{formatBytes(card.size)}</div>
          <div class="text-xs text-text-secondary mb-3">{card.count} items</div>
          <Button size="sm" variant="secondary" onclick={card.action}>Prune</Button>
        </div>
      {/each}
    </div>
    <Button variant="danger" size="sm" onclick={() => confirmPrune('Prune All', 'Remove all unused containers, images, volumes, and build cache? This cannot be undone.', doPruneAll)}>Prune All</Button>

  {:else if activeTab === 'images'}
    <div class="flex justify-end mb-4">
      <Button size="sm" variant="secondary" onclick={() => confirmPrune('Remove Dangling', 'Remove all dangling images?', () => runPrune(async () => { const res = await api.dockerPruneImages(); pruneToast(res, 'ImagesDeleted', 'Images'); await loadImages() }))}>Remove Dangling</Button>
    </div>
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
      {#if images.length === 0}
        <p class="text-sm text-text-muted">No images found.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Repository</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Tag</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Image ID</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Size</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Created</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each images as img}
                {@const parsed = parseRepoTag(img)}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 font-medium">{parsed.repo}</td>
                  <td class="py-3 px-4 text-text-secondary">{parsed.tag}</td>
                  <td class="py-3 px-4 text-text-secondary font-mono text-xs">{shortId(img.Id)}</td>
                  <td class="py-3 px-4 text-text-secondary">{formatBytes(img.Size)}</td>
                  <td class="py-3 px-4 text-text-secondary">{formatDate(img.Created)}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => imageToDelete = img.Id}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

  {:else if activeTab === 'netsvols'}
    <h2 class="text-base font-medium text-text-primary mb-4">Networks</h2>
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
      {#if networks.length === 0}
        <p class="text-sm text-text-muted">No networks found.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Name</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Driver</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Scope</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Created</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each networks as net}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 font-medium">{net.Name}</td>
                  <td class="py-3 px-4 text-text-secondary">{net.Driver}</td>
                  <td class="py-3 px-4 text-text-secondary">{net.Scope}</td>
                  <td class="py-3 px-4 text-text-secondary">{formatDate(net.Created)}</td>
                  <td class="py-3 px-4">
                    {#if systemNetworks.includes(net.Name)}
                      <Button variant="danger" size="sm" disabled>Delete</Button>
                    {:else}
                      <Button variant="danger" size="sm" onclick={() => networkToDelete = net.Id}>Delete</Button>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <div class="flex items-center justify-between mb-3">
      <h2 class="text-base font-medium text-text-primary">Volumes</h2>
      <Button size="sm" variant="secondary" onclick={() => pruneVolumesConfirm = true}>Prune Unused</Button>
    </div>
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
      {#if volumes.length === 0}
        <p class="text-sm text-text-muted">No volumes found.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Name</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Driver</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Mountpoint</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each volumes as vol}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 font-medium font-mono text-xs">{vol.Name}</td>
                  <td class="py-3 px-4 text-text-secondary">{vol.Driver}</td>
                  <td class="py-3 px-4 text-text-secondary text-xs truncate max-w-xs">{vol.Mountpoint}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => volumeToDelete = vol.Name}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  {#if pruning}
    <div class="fixed inset-0 z-50 flex items-center justify-center" role="status">
      <div class="absolute inset-0 bg-black/60 backdrop-blur-sm"></div>
      <div class="relative bg-surface-2 border border-border rounded-lg p-6 flex flex-col items-center gap-3">
        <svg class="animate-spin h-8 w-8 text-accent" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span class="text-sm text-text-primary">Pruning...</span>
      </div>
    </div>
  {/if}

  {#if pruneModal}
    <Modal
      title={pruneModal.title}
      message={pruneModal.message}
      onConfirm={() => pruneModal.action()}
      onCancel={() => pruneModal = null}
    />
  {/if}

  {#if imageToDelete}
    <Modal
      title="Remove Image"
      message="Are you sure you want to remove this image? This cannot be undone."
      onConfirm={() => deleteImage()}
      onCancel={() => imageToDelete = null}
    />
  {/if}

  {#if networkToDelete}
    <Modal
      title="Remove Network"
      message="Are you sure you want to remove this network?"
      onConfirm={() => deleteNetwork()}
      onCancel={() => networkToDelete = null}
    />
  {/if}

  {#if volumeToDelete}
    <Modal
      title="Remove Volume"
      message="Are you sure you want to remove this volume? This cannot be undone."
      onConfirm={() => deleteVolume()}
      onCancel={() => volumeToDelete = null}
    />
  {/if}

  {#if pruneVolumesConfirm}
    <Modal
      title="Prune Unused Volumes"
      message="Remove all unused volumes? This cannot be undone."
      onConfirm={() => pruneUnusedVolumes()}
      onCancel={() => pruneVolumesConfirm = false}
    />
  {/if}
</Layout>
