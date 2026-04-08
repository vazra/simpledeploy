<script>
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'

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
        await api.setup(username, password)
      }
      await api.login(username, password)
      push('/')
    } catch (err) {
      error = setupMode ? err.message : 'Invalid credentials'
    } finally {
      loading = false
    }
  }
</script>

<div class="login-container">
  <div class="login-card">
    <h1>SimpleDeploy</h1>
    <p class="subtitle">{setupMode ? 'Create Admin Account' : 'Sign In'}</p>

    {#if error}
      <div class="error">{error}</div>
    {/if}

    <form onsubmit={handleSubmit}>
      <input type="text" bind:value={username} placeholder="Username" required />
      <input type="password" bind:value={password} placeholder="Password" required />
      <button type="submit" disabled={loading}>
        {loading ? '...' : (setupMode ? 'Create Account' : 'Sign In')}
      </button>
    </form>

    <button class="link" onclick={() => setupMode = !setupMode}>
      {setupMode ? 'Back to login' : 'First time? Create admin account'}
    </button>
  </div>
</div>

<style>
  .login-container {
    display: flex; justify-content: center; align-items: center;
    min-height: 100vh; background: #0f1117;
  }
  .login-card {
    background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px;
    padding: 2rem; width: 100%; max-width: 380px;
  }
  h1 { text-align: center; color: #58a6ff; margin-bottom: 0.25rem; }
  .subtitle { text-align: center; color: #8b949e; margin-bottom: 1.5rem; font-size: 0.9rem; }
  .error { background: #3d1f1f; border: 1px solid #f85149; color: #f85149; padding: 0.5rem; border-radius: 4px; margin-bottom: 1rem; font-size: 0.85rem; }
  input {
    width: 100%; padding: 0.6rem 0.8rem; margin-bottom: 0.75rem;
    background: #0d1117; border: 1px solid #30363d; border-radius: 4px;
    color: #e1e4e8; font-size: 0.9rem;
  }
  input:focus { outline: none; border-color: #58a6ff; }
  button[type="submit"] {
    width: 100%; padding: 0.6rem; background: #238636; border: none;
    border-radius: 4px; color: #fff; font-size: 0.9rem; cursor: pointer;
  }
  button[type="submit"]:hover { background: #2ea043; }
  button[type="submit"]:disabled { opacity: 0.6; cursor: not-allowed; }
  .link {
    display: block; width: 100%; margin-top: 1rem; background: none;
    border: none; color: #58a6ff; cursor: pointer; font-size: 0.8rem; text-align: center;
  }
</style>
