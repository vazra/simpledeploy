<script>
  import { onMount, onDestroy } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let status = $state(null)
  let loading = $state(true)
  let disabled = $state(false)
  let notAdmin = $state(false)
  let syncing = $state(false)
  let copiedSHA = $state(null)
  let interval = null

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

  async function load() {
    const res = await api.gitStatus()
    if (res.status === 503) {
      disabled = true
      loading = false
      return
    }
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

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>

  {:else if notAdmin}
    <div class="bg-surface-2 rounded-xl p-6 shadow-sm border border-border/50 text-sm text-text-muted">
      Git Sync is restricted to super admins.
    </div>

  {:else if disabled}
    <div class="bg-surface-2 rounded-xl p-6 shadow-sm border border-border/50">
      <div class="flex items-start gap-3">
        <svg class="w-5 h-5 text-text-muted shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
        </svg>
        <div>
          <p class="text-sm font-medium text-text-primary">Git Sync is not enabled</p>
          <p class="text-sm text-text-muted mt-1">
            Git Sync keeps your app configs in a Git repository so changes are tracked and recoverable.
            To enable it, configure the <code class="bg-surface-3 px-1 rounded text-xs">git_sync</code> section in your SimpleDeploy config file.
          </p>
          <a
            href="https://simpledeploy.dev/docs/operations/config-sidecars/#git-sync-optional"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-1 mt-3 text-sm text-accent hover:underline"
          >
            Learn how to set up Git Sync
            <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M13.5 6H5.25A2.25 2.25 0 003 8.25v10.5A2.25 2.25 0 005.25 21h10.5A2.25 2.25 0 0018 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
            </svg>
          </a>
        </div>
      </div>
    </div>

  {:else if status}
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
</Layout>
