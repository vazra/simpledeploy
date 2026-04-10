<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import Badge from './Badge.svelte'
  import Skeleton from './Skeleton.svelte'

  let { slug } = $props()

  let events = $state([])
  let loading = $state(true)
  let expandedEvents = $state(new Set())

  onMount(async () => {
    const res = await api.getDeployEvents(slug)
    events = res.data || []
    loading = false
  })
</script>

{#if loading}
  <Skeleton type="card" count={3} />
{:else if events.length === 0}
  <p class="text-sm text-text-secondary">No deploy events yet.</p>
{:else}
  <div class="space-y-2">
    {#each events as evt, i}
      {@const variant = ['deploy', 'restart', 'pull'].includes(evt.action) ? 'success'
        : ['deploy_failed', 'restart_failed', 'pull_failed'].includes(evt.action) ? 'danger'
        : evt.action === 'rollback' ? 'warning'
        : 'info'}
      {@const hasDetail = !!evt.detail}
      {@const expanded = expandedEvents.has(i)}
      <div>
        <button
          class="flex items-center gap-3 text-sm px-3 py-2 bg-surface-1 border border-border rounded w-full text-left {hasDetail ? 'cursor-pointer hover:bg-surface-3' : 'cursor-default'}"
          onclick={() => {
            if (!hasDetail) return
            const next = new Set(expandedEvents)
            if (next.has(i)) next.delete(i)
            else next.add(i)
            expandedEvents = next
          }}
        >
          {#if hasDetail}
            <span class="text-text-muted text-xs transition-transform {expanded ? 'rotate-90' : ''}">&gt;</span>
          {/if}
          <Badge {variant}>{evt.action}</Badge>
          <span class="text-text-secondary flex-1 truncate">{hasDetail ? evt.detail.split('\n')[0] : '-'}</span>
          <span class="text-xs text-text-muted shrink-0">{evt.created_at ? new Date(evt.created_at).toLocaleString() : ''}</span>
        </button>
        {#if hasDetail && expanded}
          <div class="bg-surface-0 rounded p-3 mt-1 border border-border">
            <pre class="text-xs font-mono whitespace-pre-wrap overflow-x-auto max-h-80 overflow-y-auto text-text-secondary">{evt.detail}</pre>
          </div>
        {/if}
      </div>
    {/each}
  </div>
{/if}
