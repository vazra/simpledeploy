<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let loading = $state(true)
  let saving = $state(false)
  let changingPw = $state(false)

  let displayName = $state('')
  let email = $state('')
  let username = $state('')
  let role = $state('')
  let createdAt = $state('')

  let currentPw = $state('')
  let newPw = $state('')
  let confirmPw = $state('')
  let pwError = $state('')

  onMount(loadProfile)

  async function loadProfile() {
    loading = true
    const res = await api.getProfile()
    if (res.data) {
      displayName = res.data.display_name || ''
      email = res.data.email || ''
      username = res.data.username
      role = res.data.role
      createdAt = new Date(res.data.created_at).toLocaleDateString()
    }
    loading = false
  }

  async function saveProfile() {
    saving = true
    await api.updateProfile({ display_name: displayName, email })
    saving = false
  }

  async function changePassword() {
    pwError = ''
    if (newPw !== confirmPw) {
      pwError = 'Passwords do not match'
      return
    }
    if (!newPw) {
      pwError = 'New password required'
      return
    }
    changingPw = true
    const res = await api.changePassword({ current_password: currentPw, new_password: newPw })
    changingPw = false
    if (!res.error) {
      currentPw = ''
      newPw = ''
      confirmPw = ''
    }
  }
</script>

<Layout title="Profile">
  {#if loading}
    <div class="space-y-4 max-w-lg">
      <Skeleton class="h-10 w-full" />
      <Skeleton class="h-10 w-full" />
      <Skeleton class="h-10 w-full" />
    </div>
  {:else}
    <div class="max-w-lg space-y-8">
      <!-- Account Info (read-only) -->
      <section>
        <h2 class="text-sm font-medium text-text-secondary mb-3">Account</h2>
        <div class="bg-surface-2 rounded-lg p-4 space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-text-secondary">Username</span><span class="text-text-primary">{username}</span></div>
          <div class="flex justify-between"><span class="text-text-secondary">Role</span><span class="text-text-primary">{role}</span></div>
          <div class="flex justify-between"><span class="text-text-secondary">Created</span><span class="text-text-primary">{createdAt}</span></div>
        </div>
      </section>

      <!-- Profile -->
      <section>
        <h2 class="text-sm font-medium text-text-secondary mb-3">Profile</h2>
        <div class="space-y-3">
          <div>
            <label for="displayName" class="block text-xs text-text-secondary mb-1">Display Name</label>
            <input id="displayName" type="text" bind:value={displayName}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <div>
            <label for="email" class="block text-xs text-text-secondary mb-1">Email</label>
            <input id="email" type="email" bind:value={email}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <Button onclick={saveProfile} disabled={saving}>{saving ? 'Saving...' : 'Save Profile'}</Button>
        </div>
      </section>

      <!-- Password -->
      <section>
        <h2 class="text-sm font-medium text-text-secondary mb-3">Change Password</h2>
        <div class="space-y-3">
          <div>
            <label for="currentPw" class="block text-xs text-text-secondary mb-1">Current Password</label>
            <input id="currentPw" type="password" bind:value={currentPw}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <div>
            <label for="newPw" class="block text-xs text-text-secondary mb-1">New Password</label>
            <input id="newPw" type="password" bind:value={newPw}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <div>
            <label for="confirmPw" class="block text-xs text-text-secondary mb-1">Confirm Password</label>
            <input id="confirmPw" type="password" bind:value={confirmPw}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          {#if pwError}
            <p class="text-xs text-danger">{pwError}</p>
          {/if}
          <Button onclick={changePassword} disabled={changingPw}>{changingPw ? 'Changing...' : 'Change Password'}</Button>
        </div>
      </section>
    </div>
  {/if}
</Layout>
