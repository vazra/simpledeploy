<script>
  import { api } from '../lib/api.js'
  import { push, router } from 'svelte-spa-router'

  let { children } = $props()

  const nav = [
    { path: '/', label: 'Dashboard' },
    { path: '/alerts', label: 'Alerts' },
    { path: '/backups', label: 'Backups' },
    { path: '/users', label: 'Users' },
  ]

  async function logout() {
    await api.logout()
    push('/login')
  }
</script>

<div class="app-layout">
  <aside class="sidebar">
    <div class="logo">SimpleDeploy</div>
    <nav>
      {#each nav as item}
        <a href="#{item.path}" class:active={router.location === item.path}>{item.label}</a>
      {/each}
    </nav>
    <button class="logout" onclick={logout}>Logout</button>
  </aside>
  <main class="content">
    {@render children()}
  </main>
</div>

<style>
  .app-layout { display: flex; min-height: 100vh; }
  .sidebar {
    width: 200px; background: #161b22; border-right: 1px solid #21262d;
    display: flex; flex-direction: column; padding: 1rem 0;
  }
  .logo { padding: 0 1rem 1rem; font-size: 1.1rem; font-weight: 600; color: #58a6ff; }
  nav { flex: 1; display: flex; flex-direction: column; gap: 2px; }
  nav a {
    padding: 0.5rem 1rem; color: #8b949e; text-decoration: none; font-size: 0.85rem;
  }
  nav a:hover { color: #e1e4e8; background: #1c2128; }
  nav a.active { color: #e1e4e8; background: #1c2128; border-left: 2px solid #58a6ff; }
  .logout {
    margin: 1rem; padding: 0.4rem; background: none; border: 1px solid #30363d;
    border-radius: 4px; color: #8b949e; cursor: pointer; font-size: 0.8rem;
  }
  .logout:hover { color: #f85149; border-color: #f85149; }
  .content { flex: 1; padding: 1.5rem; overflow-y: auto; }
</style>
