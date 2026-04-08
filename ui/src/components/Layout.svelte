<script>
  import Sidebar from './Sidebar.svelte'

  let { children } = $props()
</script>

<div class="flex min-h-screen bg-surface-0">
  <Sidebar />
  <main class="flex-1 overflow-y-auto p-6">
    {@render children()}
  </main>
</div>
