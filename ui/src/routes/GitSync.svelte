<script>
  import { onMount, onDestroy } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let status = $state(null)
  let loading = $state(true)
  let notAdmin = $state(false)
  let syncing = $state(false)
  let copiedSHA = $state(null)
  let interval = null

  // Config form state
  let cfg = $state(null)
  let cfgLoading = $state(true)
  let cfgSaving = $state(false)
  let cfgError = $state('')
  let cfgSuccess = $state(false)
  let showAdvanced = $state(false)

  // Form fields
  let fEnabled = $state(false)
  let fRemote = $state('')
  let fBranch = $state('main')
  let fPollInterval = $state(60)
  let fAuthMethod = $state('none') // 'none' | 'ssh' | 'https'
  let fSSHKeyPath = $state('')
  let fHTTPSUsername = $state('git')
  let fHTTPSToken = $state('')   // empty = unchanged if https_token_set
  let fWebhookSecret = $state('') // empty = unchanged if webhook_secret_set
  let fAuthorName = $state('SimpleDeploy')
  let fAuthorEmail = $state('bot@simpledeploy.local')

  function timeAgo(isoStr) {
    if (!isoStr) return 'never'
    const diff = Date.now() - new Date(isoStr).getTime()
    const s = Math.floor(diff / 1000)
    if (s < 60) return `${s}s ago`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m} min ago`
    const h = Math.floor(m / 60)
    if (h < 24) return `${h}h ago`
    return `${Math.floor(h / 24)}d ago`
  }

  function shortSHA(sha) {
    return sha ? sha.slice(0, 8) : ''
  }

  function applyCfg(c) {
    cfg = c
    fEnabled = c.enabled
    fRemote = c.remote || ''
    fBranch = c.branch || 'main'
    fPollInterval = c.poll_interval_seconds || 60
    fSSHKeyPath = c.ssh_key_path || ''
    fHTTPSUsername = c.https_username || 'git'
    fAuthorName = c.author_name || 'SimpleDeploy'
    fAuthorEmail = c.author_email || 'bot@simpledeploy.local'
    fHTTPSToken = ''
    fWebhookSecret = ''
    if (c.ssh_key_path) {
      fAuthMethod = 'ssh'
    } else if (c.https_token_set || c.https_username) {
      fAuthMethod = 'https'
    } else {
      fAuthMethod = 'none'
    }
  }

  async function loadConfig() {
    cfgLoading = true
    const res = await api.gitConfig()
    cfgLoading = false
    if (res.data) applyCfg(res.data)
  }

  async function load() {
    const res = await api.gitStatus()
    if (res.status === 403) {
      notAdmin = true
      loading = false
      return
    }
    if (res.data) {
      status = res.data
    }
    loading = false
  }

  async function saveConfig() {
    cfgError = ''
    cfgSuccess = false
    cfgSaving = true

    const payload = {
      enabled: fEnabled,
      remote: fRemote,
      branch: fBranch,
      poll_interval_seconds: Number(fPollInterval),
      author_name: fAuthorName,
      author_email: fAuthorEmail,
      ssh_key_path: fAuthMethod === 'ssh' ? fSSHKeyPath : '',
      https_username: fAuthMethod === 'https' ? fHTTPSUsername : '',
      // null = keep existing; "" = clear; "x" = set new value
      webhook_secret: fWebhookSecret !== '' ? fWebhookSecret : null,
      https_token: fAuthMethod === 'https' && fHTTPSToken !== '' ? fHTTPSToken : null,
    }

    const res = await api.gitConfigUpdate(payload)
    cfgSaving = false
    if (res.error) {
      cfgError = res.error
      return
    }
    cfgSuccess = true
    setTimeout(() => { cfgSuccess = false }, 3000)
    await loadConfig()
    await load()
  }

  async function syncNow() {
    syncing = true
    const res = await api.gitSyncNow()
    if (res.error && res.status === 404) {
      console.warn('[GitSync] POST /api/git/sync-now not found on server')
    }
    await load()
    syncing = false
  }

  async function copyToClipboard(sha) {
    try {
      await navigator.clipboard.writeText(sha)
      copiedSHA = sha
      setTimeout(() => { copiedSHA = null }, 2000)
    } catch {
      // fallback: no-op
    }
  }

  onMount(() => {
    loadConfig()
    load()
    interval = setInterval(load, 15000)
  })

  onDestroy(() => {
    if (interval) clearInterval(interval)
  })
</script>

<Layout>
  <div class="mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Git Sync</h1>
    <p class="text-sm text-text-muted mt-1">Mirror your app config to a Git repository</p>
  </div>

  {#if loading && cfgLoading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>

  {:else if notAdmin}
    <div class="bg-surface-2 rounded-xl p-6 shadow-sm border border-border/50 text-sm text-text-muted">
      Git Sync is restricted to super admins.
    </div>

  {:else}
    <!-- Configuration card -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-4">
      <h2 class="text-sm font-medium text-text-primary mb-4">Configuration</h2>

      {#if cfgLoading}
        <Skeleton type="card" count={1} />
      {:else}
        <form onsubmit={(e) => { e.preventDefault(); saveConfig() }} class="space-y-4">
          <!-- Enable toggle -->
          <label class="flex items-center gap-3 cursor-pointer">
            <input type="checkbox" bind:checked={fEnabled} class="w-4 h-4 accent-accent" />
            <span class="text-sm font-medium text-text-primary">Enable Git Sync</span>
          </label>

          {#if fEnabled}
            <!-- Remote URL -->
            <div>
              <label class="block text-xs text-text-muted mb-1" for="git-remote">Remote URL <span class="text-red-400">*</span></label>
              <input
                id="git-remote"
                type="text"
                bind:value={fRemote}
                required
                placeholder="git@github.com:owner/repo.git"
                class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
              />
            </div>

            <!-- Branch + Poll interval -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label class="block text-xs text-text-muted mb-1" for="git-branch">Branch</label>
                <input
                  id="git-branch"
                  type="text"
                  bind:value={fBranch}
                  placeholder="main"
                  class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                />
              </div>
              <div>
                <label class="block text-xs text-text-muted mb-1" for="git-poll">Poll interval (seconds)</label>
                <input
                  id="git-poll"
                  type="number"
                  bind:value={fPollInterval}
                  min="5"
                  placeholder="60"
                  class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                />
              </div>
            </div>

            <!-- Auth method -->
            <div>
              <p class="text-xs text-text-muted mb-2">Authentication</p>
              <div class="flex gap-4 text-sm">
                {#each [['none','None (public repo)'],['ssh','SSH key'],['https','HTTPS token']] as [val, label]}
                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input type="radio" bind:group={fAuthMethod} value={val} class="accent-accent" />
                    <span class="text-text-secondary">{label}</span>
                  </label>
                {/each}
              </div>
            </div>

            {#if fAuthMethod === 'ssh'}
              <div>
                <label class="block text-xs text-text-muted mb-1" for="git-ssh-key">SSH key path</label>
                <input
                  id="git-ssh-key"
                  type="text"
                  bind:value={fSSHKeyPath}
                  placeholder="/run/secrets/git_ssh_key"
                  class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                />
              </div>
            {:else if fAuthMethod === 'https'}
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label class="block text-xs text-text-muted mb-1" for="git-https-user">HTTPS username</label>
                  <input
                    id="git-https-user"
                    type="text"
                    bind:value={fHTTPSUsername}
                    placeholder="git"
                    class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                </div>
                <div>
                  <label class="block text-xs text-text-muted mb-1" for="git-https-token">
                    HTTPS token
                    {#if cfg?.https_token_set && fHTTPSToken === ''}
                      <span class="ml-1 inline-flex items-center rounded bg-green-500/15 px-1.5 py-0.5 text-[10px] font-medium text-green-400">configured</span>
                    {/if}
                  </label>
                  <input
                    id="git-https-token"
                    type="password"
                    bind:value={fHTTPSToken}
                    placeholder={cfg?.https_token_set ? 'Leave blank to keep existing' : 'Token or password'}
                    class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                  />
                </div>
              </div>
            {/if}

            <!-- Webhook secret -->
            <div>
              <label class="block text-xs text-text-muted mb-1" for="git-webhook-secret">
                Webhook secret
                {#if cfg?.webhook_secret_set && fWebhookSecret === ''}
                  <span class="ml-1 inline-flex items-center rounded bg-green-500/15 px-1.5 py-0.5 text-[10px] font-medium text-green-400">configured</span>
                {/if}
              </label>
              <input
                id="git-webhook-secret"
                type="password"
                bind:value={fWebhookSecret}
                placeholder={cfg?.webhook_secret_set ? 'Leave blank to keep existing' : 'Optional HMAC secret for push webhook'}
                class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
              />
              <p class="text-xs text-text-muted mt-1">Used to verify GitHub/GitLab push webhook signatures.</p>
            </div>

            <!-- Advanced (author) -->
            <div>
              <button
                type="button"
                onclick={() => { showAdvanced = !showAdvanced }}
                class="text-xs text-accent hover:underline"
              >
                {showAdvanced ? 'Hide' : 'Show'} advanced settings
              </button>
              {#if showAdvanced}
                <div class="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label class="block text-xs text-text-muted mb-1" for="git-author-name">Commit author name</label>
                    <input
                      id="git-author-name"
                      type="text"
                      bind:value={fAuthorName}
                      placeholder="SimpleDeploy"
                      class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                    />
                  </div>
                  <div>
                    <label class="block text-xs text-text-muted mb-1" for="git-author-email">Commit author email</label>
                    <input
                      id="git-author-email"
                      type="text"
                      bind:value={fAuthorEmail}
                      placeholder="bot@simpledeploy.local"
                      class="w-full rounded-lg border border-border bg-surface-3 px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-1 focus:ring-accent"
                    />
                  </div>
                </div>
              {/if}
            </div>
          {/if}

          {#if cfgError}
            <p class="text-sm text-red-400">{cfgError}</p>
          {/if}
          {#if cfgSuccess}
            <p class="text-sm text-green-400">Configuration saved.</p>
          {/if}

          <div class="flex gap-2 pt-1">
            <Button type="submit" size="sm" disabled={cfgSaving}>
              {cfgSaving ? 'Saving...' : 'Save'}
            </Button>
            {#if cfg?.source === 'db'}
              <Button type="button" size="sm" variant="secondary" onclick={async () => { await api.gitDisable(); await loadConfig(); await load() }}>
                Reset to defaults
              </Button>
            {/if}
          </div>
        </form>
      {/if}
    </div>

    <!-- Status section (only when enabled) -->
    {#if !loading && status?.Enabled}
    <!-- Status card -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-4">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
        <h2 class="text-sm font-medium text-text-primary">Sync Status</h2>
        <Button size="sm" variant="secondary" onclick={syncNow} disabled={syncing}>
          {syncing ? 'Syncing...' : 'Sync now'}
        </Button>
      </div>

      <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 text-sm">
        <div>
          <dt class="text-xs text-text-muted mb-0.5">Remote</dt>
          <dd class="text-text-primary font-medium break-all">{status.Remote || '—'}</dd>
        </div>
        <div>
          <dt class="text-xs text-text-muted mb-0.5">Branch</dt>
          <dd class="text-text-primary font-medium">{status.Branch || '—'}</dd>
        </div>
        <div>
          <dt class="text-xs text-text-muted mb-0.5">Last sync</dt>
          <dd class="text-text-secondary">{timeAgo(status.LastSyncAt)}</dd>
        </div>
        <div>
          <dt class="text-xs text-text-muted mb-0.5">
            Current commit ID
            <span class="ml-1 text-[10px] bg-surface-3 rounded px-1 py-0.5 align-middle" title="The short git commit hash (first 8 characters) identifying the latest synced change">?</span>
          </dt>
          <dd class="text-text-secondary font-mono">{shortSHA(status.HeadSHA) || '—'}</dd>
        </div>
        <div>
          <dt class="text-xs text-text-muted mb-0.5">Pending commits</dt>
          <dd class="text-text-secondary">{status.PendingCommits ?? 0}</dd>
        </div>
      </dl>

      {#if status.LastSyncError}
        <div class="mt-4 flex items-start gap-2 rounded-lg bg-red-500/10 border border-red-500/20 px-3 py-2.5 text-sm text-red-400 light:bg-red-50 light:border-red-100 light:text-red-600">
          <svg class="w-4 h-4 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
          </svg>
          <span class="break-all">{status.LastSyncError}</span>
        </div>
      {/if}
    </div>

    <!-- Recent activity -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-4">
      <div class="mb-4">
        <h2 class="text-sm font-medium text-text-primary">Recent activity</h2>
        <p class="text-xs text-text-muted mt-0.5">Last 20 commits on this branch</p>
      </div>
      {#if status.RecentCommits && status.RecentCommits.length > 0}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">When</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Who</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Message</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Commit ID</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/30">
              {#each status.RecentCommits as c}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 text-text-secondary text-xs whitespace-nowrap">{timeAgo(c.When)}</td>
                  <td class="py-3 px-4 text-text-secondary text-xs whitespace-nowrap">
                    {c.AuthorName}
                    {#if c.BotCommit}
                      <span class="ml-1 inline-flex items-center rounded bg-accent/15 px-1.5 py-0.5 text-[10px] font-medium text-accent" title="Committed by SimpleDeploy bot">bot</span>
                    {/if}
                  </td>
                  <td class="py-3 px-4 text-text-primary text-xs max-w-xs truncate" title={c.Subject}>{c.Subject}</td>
                  <td class="py-3 px-4">
                    <button
                      onclick={() => copyToClipboard(c.SHA)}
                      class="font-mono text-xs text-text-secondary hover:text-accent transition-colors"
                      title="Click to copy full commit ID"
                    >
                      {copiedSHA === c.SHA ? 'Copied!' : c.ShortSHA}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <p class="text-sm text-text-muted">No commits yet. Commits will appear here once git sync has something to push.</p>
      {/if}
    </div>

    <!-- Conflicts table -->
    {#if status.RecentConflicts && status.RecentConflicts.length > 0}
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h2 class="text-sm font-medium text-text-primary mb-4">Recent Conflicts</h2>
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border/50">
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">File</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Remote commit ID</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Resolved</th>
                <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Notes</th>
                <th class="py-3 px-4"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/30">
              {#each status.RecentConflicts as c}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 font-mono text-xs text-text-primary break-all">{c.Path}</td>
                  <td class="py-3 px-4 font-mono text-xs text-text-secondary">{shortSHA(c.RemoteSHA)}</td>
                  <td class="py-3 px-4 text-text-secondary text-xs">{timeAgo(c.ResolvedAt)}</td>
                  <td class="py-3 px-4 text-text-secondary text-xs">{c.Description || '—'}</td>
                  <td class="py-3 px-4">
                    <button
                      onclick={() => copyToClipboard(c.RemoteSHA)}
                      class="text-xs text-accent hover:underline whitespace-nowrap"
                      title="Copy full commit ID to clipboard"
                    >
                      {copiedSHA === c.RemoteSHA ? 'Copied!' : 'Copy ID'}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {:else}
      <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
        <h2 class="text-sm font-medium text-text-primary mb-2">Recent Conflicts</h2>
        <p class="text-sm text-text-muted">No conflicts recorded.</p>
      </div>
    {/if}
    {/if}
  {/if}
</Layout>
