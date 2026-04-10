<script>
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import Button from '../components/Button.svelte'

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let loading = $state(false)
  let setupMode = $state(false)

  async function handleSubmit(e) {
    e.preventDefault()
    error = ''
    loading = true
    try {
      if (setupMode) {
        const res = await api.setup(username, password)
        if (res.error) { error = res.error; loading = false; return }
      }
      const res = await api.login(username, password)
      if (res.error) { error = setupMode ? res.error : 'Invalid credentials'; loading = false; return }
      push('/')
    } catch (err) {
      error = err.message
    } finally {
      loading = false
    }
  }
</script>

<div class="flex items-center justify-center min-h-screen bg-surface-0 px-4">
  <div class="w-full max-w-sm">
    <div class="bg-surface-2 border border-border rounded-xl p-8 shadow-lg">
      <div class="flex flex-col items-center mb-8">
        <svg class="w-10 h-10 text-accent mb-3" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
        </svg>
        <h1 class="text-xl font-bold text-accent">SimpleDeploy</h1>
        <p class="text-sm text-text-secondary mt-1">{setupMode ? 'Create Admin Account' : 'Sign in to continue'}</p>
      </div>

      {#if error}
        <div class="bg-red-900/20 border border-danger rounded-md px-3 py-2 mb-4 text-sm text-danger light:bg-red-50">
          {error}
        </div>
      {/if}

      <form onsubmit={handleSubmit} class="flex flex-col gap-4">
        <div>
          <label for="username" class="block text-xs font-medium text-text-secondary mb-1.5">Username</label>
          <input
            id="username"
            type="text"
            bind:value={username}
            required
            class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
          />
        </div>
        <div>
          <label for="password" class="block text-xs font-medium text-text-secondary mb-1.5">Password</label>
          <input
            id="password"
            type="password"
            bind:value={password}
            required
            class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
          />
        </div>
        <Button type="submit" {loading} variant="primary" size="md">
          {setupMode ? 'Create Account' : 'Sign In'}
        </Button>
      </form>

      <button
        onclick={() => { setupMode = !setupMode; error = '' }}
        class="block w-full mt-4 text-center text-xs text-accent hover:underline"
      >
        {setupMode ? 'Back to login' : 'First time? Create admin account'}
      </button>
    </div>
  </div>
</div>
