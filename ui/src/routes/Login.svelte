<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import Button from '../components/Button.svelte'

  let username = $state('')
  let password = $state('')
  let confirmPassword = $state('')
  let displayName = $state('')
  let email = $state('')
  let error = $state('')
  let loading = $state(false)
  let setupMode = $state(false)
  let checking = $state(true)

  const strengthLabel = $derived.by(() => {
    const len = password.length
    if (len === 0) return { text: '', color: '', width: '0%' }
    if (len < 8) return { text: 'Too short', color: 'bg-red-500', width: '25%' }
    const hasUpper = /[A-Z]/.test(password)
    const hasLower = /[a-z]/.test(password)
    const hasNum = /[0-9]/.test(password)
    const hasSpecial = /[^A-Za-z0-9]/.test(password)
    const score = [hasUpper, hasLower, hasNum, hasSpecial].filter(Boolean).length
    if (len >= 12 && score >= 3) return { text: 'Strong', color: 'bg-green-500', width: '100%' }
    if (len >= 8 && score >= 2) return { text: 'Good', color: 'bg-yellow-500', width: '66%' }
    return { text: 'Weak', color: 'bg-orange-500', width: '40%' }
  })

  onMount(() => {
    api.setupStatus().then(res => {
      setupMode = res.data?.needs_setup === true
    }).finally(() => { checking = false })
  })

  async function handleSubmit(e) {
    e.preventDefault()
    error = ''

    if (setupMode) {
      if (password !== confirmPassword) { error = 'Passwords do not match'; return }
      if (password.length < 8) { error = 'Password must be at least 8 characters'; return }
    }

    loading = true
    try {
      if (setupMode) {
        const res = await api.setup(username, password, displayName, email)
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

  const inputClass = 'w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50'
</script>

<div class="flex items-center justify-center min-h-screen bg-surface-0 px-4" style="background: radial-gradient(ellipse at top, rgba(59,130,246,0.08) 0%, transparent 50%)">
  <div class="w-full" class:max-w-sm={!setupMode} class:max-w-md={setupMode}>
    {#if checking}
      <div class="flex justify-center"><div class="w-8 h-8 border-2 border-accent/30 border-t-accent rounded-full animate-spin"></div></div>
    {:else}
    <div class="bg-surface-2 border border-border/50 rounded-2xl p-8 shadow-2xl animate-fade-in-up">
      <div class="flex flex-col items-center mb-6">
        <svg class="w-10 h-10 text-accent mb-3" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
        </svg>
        {#if setupMode}
          <h1 class="text-xl font-bold text-text-primary">Welcome to SimpleDeploy</h1>
          <p class="text-sm text-text-muted mt-2 text-center max-w-xs">Create your admin account to get started. You'll manage deployments, users, and settings from here.</p>
        {:else}
          <h1 class="text-xl font-bold text-text-primary">SimpleDeploy</h1>
          <p class="text-sm text-text-muted mt-2">Sign in to continue</p>
        {/if}
      </div>

      {#if error}
        <div class="bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3 mb-4 text-sm text-red-400 light:bg-red-50">
          {error}
        </div>
      {/if}

      <form onsubmit={handleSubmit} class="flex flex-col gap-4">
        {#if setupMode}
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label for="displayName" class="block text-xs font-medium text-text-muted mb-2">Full Name</label>
              <input id="displayName" type="text" bind:value={displayName} required class={inputClass} placeholder="Jane Doe" />
            </div>
            <div>
              <label for="email" class="block text-xs font-medium text-text-muted mb-2">Email</label>
              <input id="email" type="email" bind:value={email} required class={inputClass} placeholder="jane@example.com" />
            </div>
          </div>
        {/if}

        <div>
          <label for="username" class="block text-xs font-medium text-text-muted mb-2">Username</label>
          <input id="username" type="text" bind:value={username} required class={inputClass} />
        </div>

        <div>
          <label for="password" class="block text-xs font-medium text-text-muted mb-2">Password</label>
          <input id="password" type="password" bind:value={password} required minlength={setupMode ? 8 : undefined} class={inputClass} />
          {#if setupMode && password.length > 0}
            <div class="mt-2 flex items-center gap-2">
              <div class="flex-1 h-1.5 bg-surface-0 rounded-full overflow-hidden">
                <div class="{strengthLabel.color} h-full rounded-full transition-all duration-300" style="width: {strengthLabel.width}"></div>
              </div>
              <span class="text-xs text-text-muted">{strengthLabel.text}</span>
            </div>
          {/if}
        </div>

        {#if setupMode}
          <div>
            <label for="confirmPassword" class="block text-xs font-medium text-text-muted mb-2">Confirm Password</label>
            <input id="confirmPassword" type="password" bind:value={confirmPassword} required class={inputClass} />
            {#if confirmPassword.length > 0 && password !== confirmPassword}
              <p class="text-xs text-red-400 mt-1">Passwords do not match</p>
            {/if}
          </div>
        {/if}

        <Button type="submit" {loading} variant="primary" size="md">
          {setupMode ? 'Create Account' : 'Sign In'}
        </Button>
      </form>

    </div>
    {/if}
  </div>
</div>
