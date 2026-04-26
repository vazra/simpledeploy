<script>
  import Badge from './Badge.svelte'
  import { api } from '../lib/api.js'
  import { timeAgo } from '../lib/format.js'

  let { entry, expandable = false, compact = false, showAppColumn = false } = $props()

  let expanded = $state(false)
  let loading = $state(false)
  let fullEntry = $state(null)

  // Reset cache when the entry ID changes (e.g. row recycled or entry updated).
  $effect(() => {
    entry.id
    expanded = false
    fullEntry = null
  })

  const greenActions = new Set(['added', 'deploy_succeeded', 'login_succeeded', 'created', 'started'])
  const amberActions = new Set(['changed', 'renamed', 'scaled', 'password_changed', 'restarted', 'rollback', 'public_host_changed'])
  const redActions = new Set(['removed', 'deploy_failed', 'login_failed', 'stopped'])

  function actionVariant(action) {
    if (greenActions.has(action)) return 'success'
    if (amberActions.has(action)) return 'warning'
    if (redActions.has(action)) return 'danger'
    return 'default'
  }

  function categoryIcon(category) {
    const icons = {
      compose: '⚙',
      deploy: '🚀',
      app: '📦',
      auth: '🔑',
      user: '👤',
      backup: '💾',
      system: '🖥',
    }
    return icons[category] || '•'
  }

  async function toggleExpand() {
    if (expanded) {
      expanded = false
      return
    }
    if (fullEntry) {
      expanded = true
      return
    }
    loading = true
    const res = await api.getActivity(entry.id)
    fullEntry = res.data ?? res
    loading = false
    expanded = true
  }

  let absTime = $derived(entry.created_at ? new Date(entry.created_at).toLocaleString() : '')
  let relTime = $derived(timeAgo(entry.created_at))

  function syncBadgeProps(status) {
    if (status === 'synced') return { variant: 'success', label: '✓ synced' }
    if (status === 'pending') return { variant: 'warning', label: '⏳ pending' }
    if (status === 'failed') return { variant: 'danger', label: '✗ sync failed' }
    return null
  }

  let syncBadge = $derived(syncBadgeProps(entry.sync_status))
</script>

<div data-testid="activity-row" class="flex items-start gap-3 {compact ? 'py-1.5' : 'py-3'} px-3 bg-surface-1 border border-border/30 rounded-lg hover:bg-surface-2 transition-colors">
  {#if !compact}
    <span class="text-base shrink-0 mt-0.5" aria-hidden="true">{categoryIcon(entry.category)}</span>
  {/if}

  <div class="flex-1 min-w-0">
    <div class="flex items-center gap-2 flex-wrap">
      <Badge variant={actionVariant(entry.action)}>{entry.action}</Badge>

      {#if showAppColumn && entry.app_slug}
        <a href="#/apps/{entry.app_slug}" class="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium bg-surface-3/60 text-text-secondary hover:text-accent transition-colors">
          {entry.app_slug}
        </a>
      {/if}

      {#if syncBadge}
        <span
          class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium tracking-wide {syncBadge.variant === 'success' ? 'bg-emerald-500/10 text-emerald-400 light:bg-emerald-50 light:text-emerald-700' : syncBadge.variant === 'warning' ? 'bg-amber-500/10 text-amber-400 light:bg-amber-50 light:text-amber-700' : 'bg-red-500/10 text-red-400 light:bg-red-50 light:text-red-700'}"
          title={entry.sync_status === 'synced' ? (entry.sync_commit_sha ? entry.sync_commit_sha.slice(0, 7) : '') : entry.sync_status === 'pending' ? 'Waiting for git sync to push this change.' : (entry.sync_error ?? '')}
        >
          {syncBadge.label}
        </span>
      {/if}
    </div>

    <p class="text-sm text-text-primary mt-1 {compact ? 'truncate' : ''}">{entry.summary}</p>

    {#if entry.action === 'deploy_failed' && entry.error}
      {#if entry.error.length > 100}
        <details class="mt-1">
          <summary class="text-xs text-danger cursor-pointer">{entry.error.slice(0, 100)}…</summary>
          <pre class="text-xs text-danger mt-1 whitespace-pre-wrap break-words">{entry.error}</pre>
        </details>
      {:else}
        <p class="text-xs text-red-400 light:text-red-600 mt-1">{entry.error}</p>
      {/if}
    {/if}

    {#if entry.category === 'compose' && entry.compose_version_id && entry.app_slug}
      <a
        href="#/apps/{entry.app_slug}?tab=versions&version={entry.compose_version_id}"
        class="text-xs text-accent hover:underline mt-1 inline-block"
      >
        View diff →
      </a>
    {/if}

    {#if !compact}
      <div class="flex items-center gap-2 mt-1.5">
        <span class="text-xs text-text-muted">
          {entry.actor_name || entry.actor_source || 'system'}
        </span>
        <span class="text-text-muted/50 text-xs">·</span>
        <span class="text-xs text-text-muted" title={absTime}>{relTime}</span>
      </div>
    {/if}
  </div>

  {#if expandable}
    <button
      aria-label="Show details"
      onclick={toggleExpand}
      class="shrink-0 text-text-muted hover:text-text-primary transition-colors p-1 rounded"
    >
      {#if loading}
        <span class="text-xs">…</span>
      {:else}
        <span class="text-xs transition-transform inline-block {expanded ? 'rotate-90' : ''}">›</span>
      {/if}
    </button>
  {/if}
</div>

{#if expanded && fullEntry}
  <div class="mt-1 border border-border/30 rounded-lg overflow-hidden">
    {#if fullEntry.before_json}
      <div class="px-3 pt-2 pb-1">
        <p class="text-[11px] font-medium text-text-muted mb-1 uppercase tracking-wider">Before</p>
        <pre class="text-xs font-mono text-text-secondary bg-surface-0 rounded p-2 overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap">{JSON.stringify(fullEntry.before_json, null, 2)}</pre>
      </div>
    {/if}
    {#if fullEntry.after_json}
      <div class="px-3 pt-1 pb-2">
        <p class="text-[11px] font-medium text-text-muted mb-1 uppercase tracking-wider">After</p>
        <pre class="text-xs font-mono text-text-secondary bg-surface-0 rounded p-2 overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap">{JSON.stringify(fullEntry.after_json, null, 2)}</pre>
      </div>
    {/if}
  </div>
{/if}
