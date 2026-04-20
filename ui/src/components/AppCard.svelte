<script>
  import Badge from './Badge.svelte'
  import { formatBytesShort } from '../lib/format.js'

  let { app, metrics = null } = $props()

  const statusVariant = {
    running: 'success',
    degraded: 'warning',
    unstable: 'warning',
    stopped: 'default',
    error: 'danger'
  }
</script>

<a href="#/apps/{app.Slug}" class="block bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50 hover:shadow-lg hover:-translate-y-0.5 transition-all duration-200 group">
  <div class="flex items-start justify-between mb-2">
    <div class="flex items-center gap-2 min-w-0">
      <span class="w-2 h-2 rounded-full shrink-0 ring-2 ring-surface-2 {app.Status === 'running' ? 'bg-success' : (app.Status === 'degraded' || app.Status === 'unstable') ? 'bg-warning' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
      <h3 class="text-sm font-semibold text-text-primary tracking-tight truncate group-hover:text-accent transition-colors">{app.Name}</h3>
    </div>
    <Badge variant={statusVariant[app.Status] || 'default'}>{app.Status}</Badge>
  </div>

  {#if app.Domain}
    <p class="text-xs text-accent truncate mb-4">{app.Domain}</p>
  {/if}

  {#if metrics}
    <div class="flex gap-4 pt-3 border-t border-border/30">
      <div class="flex-1">
        <div class="text-xs text-text-muted mb-0.5">CPU</div>
        <div class="h-1.5 bg-surface-3/50 rounded-full overflow-hidden">
          <div class="h-full bg-accent rounded-full" style="width: {Math.min(metrics.cpu || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-text-secondary mt-0.5">{metrics.cpu?.toFixed(1) || 0}%</div>
      </div>
      <div class="flex-1">
        <div class="text-xs text-text-muted mb-0.5">MEM</div>
        <div class="h-1.5 bg-surface-3/50 rounded-full overflow-hidden">
          <div class="h-full bg-success rounded-full" style="width: {Math.min(metrics.memPct || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-text-secondary mt-0.5">{metrics.memPct?.toFixed(1) || 0}%{#if metrics.memLimit} · {formatBytesShort(metrics.memBytes)} / {formatBytesShort(metrics.memLimit)}{/if}</div>
      </div>
    </div>
  {/if}
</a>
