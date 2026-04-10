<script>
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import Button from '../components/Button.svelte'

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let loading = $state(false)
  let setupMode = $state(false)
  let checking = $state(true)

  $effect(() => {
    api.setupStatus().then(res => {
      setupMode = res.needs_setup === true
    }).finally(() => { checking = false })
  })

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

<div class="flex items-center justify-center min-h-screen bg-surface-0 px-4" style="background: radial-gradient(ellipse at top, rgba(59,130,246,0.08) 0%, transparent 50%)">
  <div class="w-full max-w-sm">
    {#if checking}
      <div class="flex justify-center"><div class="w-8 h-8 border-2 border-accent/30 border-t-accent rounded-full animate-spin"></div></div>
    {:else}
    <div class="bg-surface-2 border border-border/50 rounded-2xl p-8 shadow-2xl animate-fade-in-up">
      <div class="flex flex-col items-center mb-8">
        <svg class="w-10 h-10 text-accent mb-3" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
        </svg>
        <h1 class="text-xl font-bold text-text-primary">SimpleDeploy</h1>
        <p class="text-sm text-text-muted mt-2">{setupMode ? 'Create Admin Account' : 'Sign in to continue'}</p>
      </div>

      {#if error}
        <div class="bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3 mb-4 text-sm text-red-400 light:bg-red-50">
          {error}
        </div>
      {/if}

      <form onsubmit={handleSubmit} class="flex flex-col gap-4">
        <div>
          <label for="username" class="block text-xs font-medium text-text-muted mb-2">Username</label>
          <input
            id="username"
            type="text"
            bind:value={username}
            required
            class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50"
          />
        </div>
        <div>
          <label for="password" class="block text-xs font-medium text-text-muted mb-2">Password</label>
          <input
            id="password"
            type="password"
            bind:value={password}
            required
            class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50"
          />
        </div>
        <Button type="submit" {loading} variant="primary" size="md">
          {setupMode ? 'Create Account' : 'Sign In'}
        </Button>
      </form>

    </div>
    {/if}
  </div>
</div>
